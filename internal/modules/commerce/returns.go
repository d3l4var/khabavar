package commerce

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Return request statuses (mirrors the CHECK constraint).
const (
	ReturnRequested = "REQUESTED"
	ReturnApproved  = "APPROVED"
	ReturnRejected  = "REJECTED"
	ReturnReceived  = "RECEIVED"
	ReturnRefunded  = "REFUNDED"
)

var (
	ErrReturnReasonRequired  = errors.New("commerce: a return request needs a reason")
	ErrReturnQuantityInvalid = errors.New("commerce: return quantity must be positive")
	ErrReturnNotAllowed      = errors.New("commerce: this order cannot be returned")
	ErrReturnAlreadyOpen     = errors.New("commerce: this order already has an open return request")
)

// ReturnRequest is the customer's claim; the money movement stays in refunds.
type ReturnRequest struct {
	ID           string    `json:"id"`
	OrderID      string    `json:"order_id"`
	CustomerID   string    `json:"customer_id"`
	Quantity     int       `json:"quantity"`
	Reason       string    `json:"reason"`
	Status       string    `json:"status"`
	DecisionNote string    `json:"decision_note,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// returnableStatuses: a return only makes sense after the money was taken and the goods
// shipped. PENDING_PAYMENT and CANCELLED are excluded on purpose.
var returnableStatuses = map[string]bool{
	OrderPaid:          true,
	OrderInFulfillment: true,
	OrderShipped:       true,
	// DELIVERED was missing: the goods are in the customer's hands, which is the most
	// ordinary moment to return them, and the route answered 409.
	OrderDelivered: true,
	OrderCompleted: true,
}

// CreateReturnRequest records a customer's return claim. It is scoped by customer: the
// order must belong to the caller, so an id from another customer answers "not found".
func (s *Service) CreateReturnRequest(ctx context.Context, orderID, customerID string, quantity int, reason string) (*ReturnRequest, error) {
	if reason == "" {
		return nil, ErrReturnReasonRequired
	}
	if quantity <= 0 {
		return nil, ErrReturnQuantityInvalid
	}
	o, err := s.store.GetOrder(ctx, orderID)
	if err != nil {
		return nil, err
	}
	if o == nil || o.CustomerID != customerID {
		return nil, ErrOrderNotFound
	}
	if !returnableStatuses[o.Status] {
		return nil, fmt.Errorf("%w: status=%s", ErrReturnNotAllowed, o.Status)
	}
	store, ok := s.store.(interface {
		CreateReturnRequest(ctx context.Context, rr ReturnRequest) (*ReturnRequest, error)
	})
	if !ok {
		return nil, errors.New("commerce: return requests are not supported by this store")
	}
	rr, err := store.CreateReturnRequest(ctx, ReturnRequest{
		OrderID: orderID, CustomerID: customerID, Quantity: quantity,
		Reason: reason, Status: ReturnRequested,
	})
	if err != nil {
		var pgErr interface{ SQLState() string }
		if errors.As(err, &pgErr) && pgErr.SQLState() == "23505" {
			return nil, ErrReturnAlreadyOpen // the partial unique index spoke
		}
		return nil, err
	}
	if s.auditor != nil {
		// A failure here is returned, never swallowed: a claim with no audit trail is
		// worse than a visible error.
		if err := s.auditor.Record(ctx, "CUSTOMER", customerID, "return.request", "return_request", rr.ID,
			nil, map[string]any{"order_id": orderID, "quantity": quantity, "reason": reason}, ""); err != nil {
			return nil, fmt.Errorf("audit return request: %w", err)
		}
	}
	return rr, nil
}

// CreateReturnRequest inserts the claim row.
func (s *DBStore) CreateReturnRequest(ctx context.Context, rr ReturnRequest) (*ReturnRequest, error) {
	var created pgtype.Timestamptz
	err := s.pool.QueryRow(ctx,
		`INSERT INTO return_requests (order_id, customer_id, quantity, reason, status)
		 VALUES ($1,$2,$3,$4,$5)
		 RETURNING id, created_at`,
		rr.OrderID, rr.CustomerID, rr.Quantity, rr.Reason, rr.Status).Scan(&rr.ID, &created)
	if err != nil {
		return nil, err
	}
	if created.Valid {
		rr.CreatedAt = created.Time
	}
	return &rr, nil
}

// ListReturnRequestsByOrder returns the order's claims, newest first.
func (s *DBStore) ListReturnRequestsByOrder(ctx context.Context, orderID string) ([]ReturnRequest, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, order_id, customer_id, quantity, reason, status, COALESCE(decision_note,''), created_at
		   FROM return_requests WHERE order_id = $1 ORDER BY created_at DESC`, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ReturnRequest, 0, 2)
	for rows.Next() {
		var rr ReturnRequest
		var created pgtype.Timestamptz
		if err := rows.Scan(&rr.ID, &rr.OrderID, &rr.CustomerID, &rr.Quantity, &rr.Reason,
			&rr.Status, &rr.DecisionNote, &created); err != nil {
			return nil, err
		}
		if created.Valid {
			rr.CreatedAt = created.Time
		}
		out = append(out, rr)
	}
	return out, rows.Err()
}

var _ = pgx.ErrNoRows
