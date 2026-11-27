package commerce

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"
)

// A return claim has five states. Creation lands in REQUESTED; everything after that is a
// staff decision, and until this file existed nothing could move a claim out of REQUESTED —
// a customer could file a return and it sat there forever.
//
//	REQUESTED → APPROVED → RECEIVED → REFUNDED
//	REQUESTED → REJECTED
var (
	ErrReturnNotFound     = errors.New("commerce: return request not found")
	ErrReturnTransition   = errors.New("commerce: invalid return transition")
	ErrReturnDecisionNote = errors.New("commerce: a rejection needs a decision note")
)

// returnTransitions is the only legal map. Anything else is a 409, never a silent write.
var returnTransitions = map[string][]string{
	ReturnRequested: {ReturnApproved, ReturnRejected},
	ReturnApproved:  {ReturnReceived},
	ReturnReceived:  {ReturnRefunded},
	ReturnRejected:  {},
	ReturnRefunded:  {},
}

// CanTransitionReturn reports whether from → to is legal.
func CanTransitionReturn(from, to string) bool {
	for _, allowed := range returnTransitions[from] {
		if allowed == to {
			return true
		}
	}
	return false
}

// TransitionReturn moves a claim from one state to the next. The UPDATE carries the expected
// source state in its WHERE clause, so two concurrent decisions cannot both succeed: the loser
// gets zero rows and a 409 rather than overwriting the winner.
func (s *DBStore) TransitionReturn(ctx context.Context, id, from, to, note string) (*ReturnRequest, error) {
	var rr ReturnRequest
	err := s.pool.QueryRow(ctx,
		`UPDATE return_requests
		    SET status = $3, decision_note = NULLIF($4,'')
		  WHERE id = $1 AND status = $2
		  RETURNING id, order_id, customer_id, quantity, reason, status,
		            COALESCE(decision_note,''), created_at`,
		id, from, to, note).Scan(&rr.ID, &rr.OrderID, &rr.CustomerID, &rr.Quantity, &rr.Reason,
		&rr.Status, &rr.DecisionNote, &rr.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		// Either the claim does not exist or it is no longer in `from`.
		var current string
		if qerr := s.pool.QueryRow(ctx, `SELECT status FROM return_requests WHERE id = $1`, id).Scan(&current); qerr != nil {
			return nil, ErrReturnNotFound
		}
		return nil, fmt.Errorf("%w: %s → %s (current %s)", ErrReturnTransition, from, to, current)
	}
	if err != nil {
		return nil, err
	}
	return &rr, nil
}

// GetReturnRequest reads one claim.
func (s *DBStore) GetReturnRequest(ctx context.Context, id string) (*ReturnRequest, error) {
	var rr ReturnRequest
	err := s.pool.QueryRow(ctx,
		`SELECT id, order_id, customer_id, quantity, reason, status, COALESCE(decision_note,''), created_at
		   FROM return_requests WHERE id = $1`, id).Scan(&rr.ID, &rr.OrderID, &rr.CustomerID,
		&rr.Quantity, &rr.Reason, &rr.Status, &rr.DecisionNote, &rr.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrReturnNotFound
	}
	if err != nil {
		return nil, err
	}
	return &rr, nil
}

// DecideReturn approves or rejects a REQUESTED claim. A rejection must say why: a customer
// whose return was refused is owed a reason (R-06 §۱۱).
func (s *Service) DecideReturn(ctx context.Context, id string, approve bool, note string) (*ReturnRequest, error) {
	to := ReturnRejected
	if approve {
		to = ReturnApproved
	} else if note == "" {
		return nil, ErrReturnDecisionNote
	}
	st, err := s.returnStore()
	if err != nil {
		return nil, err
	}
	return st.TransitionReturn(ctx, id, ReturnRequested, to, note)
}

// ReceiveReturn records that the goods came back (APPROVED → RECEIVED). It does not move
// money: the refund is a separate, audited step.
func (s *Service) ReceiveReturn(ctx context.Context, id, note string) (*ReturnRequest, error) {
	st, err := s.returnStore()
	if err != nil {
		return nil, err
	}
	return st.TransitionReturn(ctx, id, ReturnApproved, ReturnReceived, note)
}

// returnTransitioner is the store surface the return flow needs. DBStore implements it; the
// service reaches it through the same optional refund backend, so a service without a store
// answers ErrRefundStoreNotWired rather than panicking.
type returnTransitioner interface {
	TransitionReturn(ctx context.Context, id, from, to, note string) (*ReturnRequest, error)
	GetReturnRequest(ctx context.Context, id string) (*ReturnRequest, error)
}

func (s *Service) returnStore() (returnTransitioner, error) {
	if s.refunds == nil {
		return nil, ErrRefundStoreNotWired
	}
	rs, ok := s.refunds.(returnTransitioner)
	if !ok {
		return nil, ErrRefundStoreNotWired
	}
	return rs, nil
}

// ListReturns returns claims for the panel, newest first, optionally filtered by status.
// The panel had no way to find a claim: only transitions by id existed, so staff could not
// see the queue they were supposed to work.
func (s *DBStore) ListReturns(ctx context.Context, status string, page, pageSize int) ([]ReturnRequest, int, error) {
	where := ""
	args := []any{}
	if status != "" {
		where = " WHERE status = $1"
		args = append(args, status)
	}
	var total int
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM return_requests"+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := s.pool.Query(ctx,
		`SELECT id, order_id, customer_id, quantity, reason, status, COALESCE(decision_note,''), created_at
		   FROM return_requests`+where+
			` ORDER BY created_at DESC LIMIT $`+strconv.Itoa(len(args)-1)+` OFFSET $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]ReturnRequest, 0, pageSize)
	for rows.Next() {
		var rr ReturnRequest
		if err := rows.Scan(&rr.ID, &rr.OrderID, &rr.CustomerID, &rr.Quantity, &rr.Reason,
			&rr.Status, &rr.DecisionNote, &rr.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, rr)
	}
	return out, total, rows.Err()
}

// ListReturns is the service face of the panel queue.
func (s *Service) ListReturns(ctx context.Context, status string, page, pageSize int) ([]ReturnRequest, int, error) {
	type lister interface {
		ListReturns(context.Context, string, int, int) ([]ReturnRequest, int, error)
	}
	if s.refunds == nil {
		return nil, 0, ErrRefundStoreNotWired
	}
	l, ok := s.refunds.(lister)
	if !ok {
		return nil, 0, ErrRefundStoreNotWired
	}
	return l.ListReturns(ctx, status, page, pageSize)
}
