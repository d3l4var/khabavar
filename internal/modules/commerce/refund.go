// refund.go adds the refund flow + transaction reconciliation to commerce
// (R-06 §۱۰, SAD §۷.۱, MA-6 refund policy).
//
// The refund business rules:
//   - An order can be refunded only if it is PAID, IN_FULFILLMENT, PARTIALLY_SHIPPED,
//     SHIPPED, DELIVERED, or COMPLETED (i.e. payment was captured). A
//     PENDING_PAYMENT or CANCELLED order has nothing to refund.
//   - The refund amount must be > 0 and ≤ (PaidTotal − existing RefundedTotal).
//     A partial refund leaves the order in its current status; a full refund
//     (RefundedTotal reaches PaidTotal) transitions the order to REFUNDED.
//   - Each refund records a reason and enqueues a RefundIssued outbox event so
//     the notifications module can inform the customer.
//   - Transaction reconciliation (ReconcileTransaction): PaidTotal must equal
//     the sum of succeeded payment attempts MINUS the sum of refunds. This is
//     the financial integrity check run by the admin dashboard / a nightly job.
package commerce

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Additional order status (refund-specific).
const (
	OrderRefunded          = "REFUNDED"           // full refund
	OrderPartiallyRefunded = "PARTIALLY_REFUNDED" // one or more partial refunds
)

// Refund mirrors a refund record (would be a `refunds` table; for now tracked
// via the order's RefundedTotal + an audit log). A future migration adds the
// table; the Service interface is stable.
type Refund struct {
	ID          string
	OrderID     string
	Amount      int64
	Currency    string
	Reason      string
	Status      string // PENDING/SUCCEEDED/FAILED
	CreatedAt   time.Time
	ProcessedAt *time.Time
}

// Refund status constants.
const (
	RefundPending   = "PENDING"
	RefundSucceeded = "SUCCEEDED"
	RefundFailed    = "FAILED"
)

// Sentinel errors for refunds.
var (
	ErrRefundNotFound      = errors.New("commerce: refund not found")
	ErrRefundAmountInvalid = errors.New("commerce: refund amount must be > 0")
	ErrRefundExceedsPaid   = errors.New("commerce: refund amount exceeds paid − already refunded")
	ErrOrderNotRefundable  = errors.New("commerce: order is not in a refundable state")
	ErrRefundReasonReq     = errors.New("commerce: refund reason required")
)

// refundableStatuses is the set of order statuses where a refund is allowed
// (payment was captured).
var refundableStatuses = map[string]bool{
	OrderPaid:              true,
	OrderInFulfillment:     true,
	OrderPartiallyShip:     true,
	OrderShipped:           true,
	OrderDelivered:         true,
	OrderCompleted:         true,
	OrderPartiallyRefunded: true,
}

// RefundStore is the persistence boundary for refunds (separate from Store to
// avoid breaking existing tests; DBStore implements both).
type RefundStore interface {
	CreateRefund(ctx context.Context, r Refund) (string, error)
	ListRefunds(ctx context.Context, orderID string) ([]Refund, error)
	UpdateOrderRefunded(ctx context.Context, id string, refundedTotal int64, status string) error
	// SumSucceededAttempts returns the total amount of succeeded payment attempts
	// for an order (for reconciliation: PaidTotal should = this − sum refunds).
	SumSucceededAttempts(ctx context.Context, orderID string) (int64, error)
}

// CreateRefund issues a refund for an order. amount is in the order's currency
// (IRR). If the refund brings RefundedTotal to PaidTotal, the order transitions
// to REFUNDED; otherwise to PARTIALLY_REFUNDED (if not already there).
func (s *Service) CreateRefund(ctx context.Context, orderID string, amount int64, reason string) (string, error) {
	if amount <= 0 {
		return "", ErrRefundAmountInvalid
	}
	if reason == "" {
		return "", ErrRefundReasonReq
	}
	o, err := s.store.GetOrder(ctx, orderID)
	if err != nil {
		return "", err
	}
	if o == nil {
		return "", ErrOrderNotFound
	}
	if !refundableStatuses[o.Status] {
		return "", fmt.Errorf("%w: status=%s", ErrOrderNotRefundable, o.Status)
	}
	// Check the refund doesn't exceed paid − already refunded.
	available := o.PaidTotal - o.RefundedTotal
	if amount > available {
		return "", fmt.Errorf("%w: amount=%d available=%d", ErrRefundExceedsPaid, amount, available)
	}
	// Record the refund row.
	id, err := s.refunds.CreateRefund(ctx, Refund{
		OrderID:   orderID,
		Amount:    amount,
		Currency:  o.Currency,
		Reason:    reason,
		Status:    RefundSucceeded,
		CreatedAt: s.now(),
	})
	if err != nil {
		return "", err
	}
	// Update the order's RefundedTotal + status.
	newRefunded := o.RefundedTotal + amount
	var newStatus string
	if newRefunded >= o.PaidTotal {
		newStatus = OrderRefunded
	} else {
		newStatus = OrderPartiallyRefunded
	}
	if err := s.refunds.UpdateOrderRefunded(ctx, orderID, newRefunded, newStatus); err != nil {
		return "", err
	}
	// Enqueue the RefundIssued event (outbox, ADR-ARCH-007).
	s.enqueue(ctx, "RefundIssued", "order", orderID, map[string]any{
		"order_id": orderID, "amount": amount, "reason": reason, "refund_id": id,
	})
	return id, nil
}

// ListRefunds returns the refund history for an order (admin view).
func (s *Service) ListRefunds(ctx context.Context, orderID string) ([]Refund, error) {
	if orderID == "" {
		return nil, ErrOrderNotFound
	}
	return s.refunds.ListRefunds(ctx, orderID)
}

// ReconciliationResult holds the transaction reconciliation outcome for an order.
type ReconciliationResult struct {
	OrderID                 string
	PaidTotal               int64
	SucceededAttempts       int64 // sum of succeeded payment attempts
	RefundedTotal           int64
	ExpectedPaidAfterRefund int64 // SucceededAttempts − RefundedTotal
	Reconciled              bool  // PaidTotal == SucceededAttempts − RefundedTotal
}

// ReconcileTransaction checks that the order's PaidTotal equals the sum of
// succeeded payment attempts minus the sum of refunds. This is the financial
// integrity check (run by the admin dashboard / nightly job, R-06 §۱۰).
func (s *Service) ReconcileTransaction(ctx context.Context, orderID string) (*ReconciliationResult, error) {
	o, err := s.store.GetOrder(ctx, orderID)
	if err != nil {
		return nil, err
	}
	if o == nil {
		return nil, ErrOrderNotFound
	}
	succeeded, err := s.refunds.SumSucceededAttempts(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("sum attempts: %w", err)
	}
	expected := succeeded - o.RefundedTotal
	return &ReconciliationResult{
		OrderID:                 orderID,
		PaidTotal:               o.PaidTotal,
		SucceededAttempts:       succeeded,
		RefundedTotal:           o.RefundedTotal,
		ExpectedPaidAfterRefund: expected,
		Reconciled:              o.PaidTotal == expected,
	}, nil
}

// SetRefundStore wires the refund backend (DBStore implements RefundStore).
func (s *Service) SetRefundStore(rs RefundStore) {
	s.refunds = rs
}
