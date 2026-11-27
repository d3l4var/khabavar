// refund.go adds the refund flow + transaction reconciliation to commerce
// (R-06 §۱۰, SAD §۷.۱, MA-6 refund policy).
//
// The refund business rules:
//   - An order can be refunded only if payment was captured (PAID, IN_FULFILLMENT,
//     PARTIALLY_SHIPPED, SHIPPED, DELIVERED, COMPLETED). A PENDING_PAYMENT or
//     CANCELLED order has nothing to refund.
//   - The refund amount must be > 0 and ≤ (PaidTotal − existing RefundedTotal).
//     A partial refund leaves the order status untouched; a full refund closes the
//     order (CLOSED) — DB v2 §10.3 has no REFUNDED order status, so inventing one
//     would violate chk_orders_status.
//   - Every refund row points at the SUCCEEDED payment attempt it reverses
//     (refunds.payment_attempt_id NOT NULL, DB v2 §10.6) and records who approved it
//     (approved_by) plus an audit entry (IAM §۷: refunds are an audited action).
//   - Each refund enqueues a RefundIssued outbox event so the notifications module
//     can inform the customer.
//   - Transaction reconciliation (ReconcileTransaction): PaidTotal must equal the sum
//     of succeeded payment attempts MINUS the sum of refunds.
package commerce

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/khabavar/khabavar/internal/modules/shared"
)

// Refund mirrors the `refunds` table (migration 000006, DB v2 §10.6). There is no
// currency column: an order is single-currency, so the refund inherits it.
type Refund struct {
	ID                string     `json:"id"`
	OrderID           string     `json:"order_id"`
	PaymentAttemptID  string     `json:"payment_attempt_id"`
	OrderItemID       string     `json:"order_item_id"` // empty = order-level refund
	Amount            int64      `json:"amount"`
	Reason            string     `json:"reason"`
	Status            string     `json:"status"`
	ProviderReference string     `json:"provider_reference"`
	ApprovedBy        string     `json:"approved_by"` // admin_user_id (IAM §۷)
	CreatedAt         time.Time  `json:"created_at"`
	CompletedAt       *time.Time `json:"completed_at"`
}

// Refund status constants — exactly the values allowed by chk_refunds_status.
const (
	RefundRequested  = "REQUESTED"
	RefundApproved   = "APPROVED"
	RefundProcessing = "PROCESSING"
	RefundCompleted  = "COMPLETED"
	RefundRejected   = "REJECTED"
	RefundFailed     = "FAILED"
)

// Sentinel errors for refunds.
var (
	ErrRefundAmountInvalid = errors.New("commerce: refund amount must be > 0")
	ErrRefundExceedsPaid   = errors.New("commerce: refund amount exceeds paid − already refunded")
	ErrOrderNotRefundable  = errors.New("commerce: order is not in a refundable state")
	ErrRefundReasonReq     = errors.New("commerce: refund reason required")
	ErrNoSucceededPayment  = errors.New("commerce: order has no succeeded payment attempt to refund")
)

// refundableStatuses is the set of order statuses where a refund is allowed
// (payment was captured).
var refundableStatuses = map[string]bool{
	OrderPaid:          true,
	OrderInFulfillment: true,
	OrderPartiallyShip: true,
	OrderShipped:       true,
	OrderDelivered:     true,
	OrderCompleted:     true,
}

// RefundStore is the persistence boundary for refunds (separate from Store so
// existing tests keep a minimal fake; DBStore implements both).
type RefundStore interface {
	CreateRefund(ctx context.Context, r Refund) (string, error)
	ListRefunds(ctx context.Context, orderID string) ([]Refund, error)
	UpdateOrderRefunded(ctx context.Context, id string, refundedTotal int64, status string) error
	// SuccessfulAttemptID returns the id of the SUCCEEDED payment attempt for an
	// order, which a refund must reference (DB v2 §10.6).
	SuccessfulAttemptID(ctx context.Context, orderID string) (string, error)
	// SumSucceededAttempts returns the total of succeeded payment attempts for an
	// order (reconciliation: PaidTotal should equal this minus refunds).
	SumSucceededAttempts(ctx context.Context, orderID string) (int64, error)
}

// AuditRecorder records a sensitive action (IAM §۷). *audit.Recorder satisfies it;
// declaring the interface here keeps commerce free of an audit import.
type AuditRecorder interface {
	Record(ctx context.Context, actorType, actorID, action, entityType, entityID string, oldValues, newValues any, ip string) error
}

// SetAuditor wires the audit recorder (nil = no audit writes).
func (s *Service) SetAuditor(a AuditRecorder) { s.auditor = a }

// CreateRefund issues a refund for an order. amount is in IRR. The refund row is
// written as COMPLETED because the payment adapter is mock-first (ADR-ARCH-011): the
// provider call is synchronous and cannot fail in the mock, so there is no async
// settlement step to wait for. A real adapter will insert REQUESTED and settle later.
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
	available := o.PaidTotal - o.RefundedTotal
	if amount > available {
		return "", fmt.Errorf("%w: amount=%d available=%d", ErrRefundExceedsPaid, amount, available)
	}
	attemptID, err := s.refunds.SuccessfulAttemptID(ctx, orderID)
	if err != nil {
		return "", err
	}
	if attemptID == "" {
		return "", ErrNoSucceededPayment
	}

	now := s.now()
	id, err := s.refunds.CreateRefund(ctx, Refund{
		OrderID:          orderID,
		PaymentAttemptID: attemptID,
		Amount:           amount,
		Reason:           reason,
		Status:           RefundCompleted,
		ApprovedBy:       shared.ActorID(ctx),
		CreatedAt:        now,
		CompletedAt:      &now,
	})
	if err != nil {
		return "", err
	}

	// A full refund closes the order; a partial one leaves the status alone
	// (DB v2 §10.3 has no REFUNDED status).
	newRefunded := o.RefundedTotal + amount
	newStatus := o.Status
	if newRefunded >= o.PaidTotal {
		newStatus = OrderClosed
	}
	if err := s.refunds.UpdateOrderRefunded(ctx, orderID, newRefunded, newStatus); err != nil {
		return "", err
	}

	// Refunds are an audited action (IAM §۷). The refund row is already written, so a
	// failure here cannot undo the money movement — but it must not be silent either: the
	// error is returned and the caller reports it alongside the refund id.
	if s.auditor != nil {
		if aerr := s.auditor.Record(ctx, "ADMIN", shared.ActorID(ctx), "order.refund.issued", "order", orderID,
			map[string]any{"refunded_total": o.RefundedTotal, "status": o.Status},
			map[string]any{"refunded_total": newRefunded, "status": newStatus, "amount": amount, "refund_id": id},
			""); aerr != nil {
			return id, fmt.Errorf("refund %s issued but audit failed: %w", id, aerr)
		}
	}

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
//
// paid_total and refunded_total are independent accumulators (DB v2 §10.3): a refund
// does not lower paid_total, it raises refunded_total. The integrity check is therefore
// paid_total == Σ succeeded attempts; NetPaid is reported for the dashboard.
type ReconciliationResult struct {
	OrderID           string
	PaidTotal         int64
	SucceededAttempts int64 // sum of succeeded payment attempts
	RefundedTotal     int64
	NetPaid           int64 // PaidTotal − RefundedTotal
	Reconciled        bool  // PaidTotal == SucceededAttempts
}

// ReconcileTransaction checks that the order's PaidTotal equals the sum of succeeded
// payment attempts (R-06 §۱۰ financial integrity check, run by the admin dashboard or
// a nightly job). A mismatch means money was recorded without a captured payment, or
// the reverse.
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
	return &ReconciliationResult{
		OrderID:           orderID,
		PaidTotal:         o.PaidTotal,
		SucceededAttempts: succeeded,
		RefundedTotal:     o.RefundedTotal,
		NetPaid:           o.PaidTotal - o.RefundedTotal,
		Reconciled:        o.PaidTotal == succeeded,
	}, nil
}

// SetRefundStore wires the refund backend (DBStore implements RefundStore).
func (s *Service) SetRefundStore(rs RefundStore) {
	s.refunds = rs
}
