// http.go mounts the commerce HTTP endpoints (R-07 §commerce). The handlers are
// thin: they decode the request, call the Service, and write the JSON response.
// All domain errors are mapped to HTTP status codes here, never in the Service.
package commerce

import (
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/khabavar/khabavar/internal/modules/authorization"
)

// ── Error → status mapping ────────────────────────────────────────────────────

func statusForError(err error) int {
	switch {
	case errors.Is(err, ErrCustomerSessionRequired):
		return http.StatusUnauthorized
	case errors.Is(err, ErrVariantRequired):
		return http.StatusBadRequest
	case errors.Is(err, ErrOrderNotFound), errors.Is(err, ErrCartNotFound):
		return http.StatusNotFound
	case errors.Is(err, ErrCartEmpty),
		errors.Is(err, ErrTotalsMismatch),
		errors.Is(err, ErrIntentExists),
		errors.Is(err, ErrIntentNotSucceeded),
		errors.Is(err, ErrAlreadyPaid),
		errors.Is(err, ErrCannotCancel),
		errors.Is(err, ErrContactRequired),
		errors.Is(err, ErrOrderNumberRequired),
		errors.Is(err, ErrRefundAmountInvalid),
		errors.Is(err, ErrRefundReasonReq),
		// Return-request validation: a bad request from the client must not surface as a
		// 500, which would pollute the server-error signal.
		errors.Is(err, ErrReturnReasonRequired),
		errors.Is(err, ErrReturnQuantityInvalid):
		return http.StatusBadRequest
	case errors.Is(err, ErrRefundExceedsPaid), errors.Is(err, ErrOrderNotRefundable),
		errors.Is(err, ErrReturnNotAllowed), errors.Is(err, ErrReturnAlreadyOpen):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

type errorResponse struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

// writeError maps a domain error to a stable HTTP response. Infrastructure failures
// (anything outside the domain sentinels) become a generic 500: SQL text, table names
// and driver state must never reach a client. The detail goes to the log instead.
func writeError(w http.ResponseWriter, err error) {
	if code, name := cartErrorCode(err); code != 0 {
		writeJSON(w, code, map[string]any{"error": err.Error(), "code": name})
		return
	}
	code := statusForError(err)
	if code >= http.StatusInternalServerError {
		slog.Error("commerce: request failed", "err", err)
		writeJSON(w, code, errorResponse{Error: "internal error", Code: "internal_error"})
		return
	}
	writeJSON(w, code, errorResponse{Error: err.Error(), Code: errorCode(err)})
}

// errorCode derives a stable machine-readable code from a domain error, dropping the
// detail suffix (": status=X") so the code does not vary per request.
func errorCode(err error) string {
	msg := err.Error()
	if i := strings.IndexByte(msg, ':'); i > 0 {
		msg = msg[:i]
	}
	return strings.ToUpper(strings.NewReplacer(" ", "_", "-", "_").Replace(msg))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// ── Refund handlers (P6) ──────────────────────────────────────────────────────

// CreateRefundHandler serves POST /api/v1/admin/orders/{id}/refunds — issue a
// refund for an order (R-06 §۱۰). amount is in IRR; reason is mandatory.
func CreateRefundHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orderID := r.PathValue("id")
		var req struct {
			Amount int64  `json:"amount"`
			Reason string `json:"reason"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid json"})
			return
		}
		id, err := svc.CreateRefund(r.Context(), orderID, req.Amount, req.Reason)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"id": id})
	}
}

// ListRefundsHandler serves GET /api/v1/admin/orders/{id}/refunds — the refund
// history for an order (admin view).
func ListRefundsHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orderID := r.PathValue("id")
		refunds, err := svc.ListRefunds(r.Context(), orderID)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": refunds})
	}
}

// ReconcileTransactionHandler serves GET /api/v1/admin/orders/{id}/reconcile —
// the transaction integrity check (PaidTotal vs succeeded attempts − refunds).
func ReconcileTransactionHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orderID := r.PathValue("id")
		res, err := svc.ReconcileTransaction(r.Context(), orderID)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": res})
	}
}

// ── CreateOrder ───────────────────────────────────────────────────────────────

// CreateOrderHandler handles POST /api/v1/orders.
//
// Request body: OrderInput (JSON). The caller resolves variant snapshots and unit
// prices from catalog/pricing before calling this endpoint.
func CreateOrderHandler(svc *Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in OrderInput
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeError(w, errors.New("commerce: invalid JSON body"))
			return
		}
		in.OrderNumber = NormalizeOrderNumber(in.OrderNumber)
		oid, err := svc.CreateOrder(r.Context(), in)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"id": oid})
	})
}

// ── GetOrderDetail ────────────────────────────────────────────────────────────

// GetOrderHandler handles GET /api/v1/orders/{id}. The path segment may be either the
// internal UUID or the business order number (KHA-ORD-YYYY-NNNNNN) — the contract exposes
// GET /orders/{orderNumber} for guest tracking (UJ-01).
//
// Authorisation is part of the contract (R-07): a customer reads only their own order, and
// a guest order requires ?phone= matching guest_phone, otherwise 403. Without this gate a
// sequential order number is enough to read someone else's phone number and address.
func GetOrderHandler(svc *Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			id = strings.TrimPrefix(r.URL.Path, "/api/v1/orders/")
		}
		// Resolve the path segment to an order id: a UUID is used as-is, anything else is
		// the business order number (KHA-ORD-YYYY-NNNNNN).
		orderID := id
		if _, perr := uuid.Parse(id); perr != nil {
			o, err := svc.GetOrderByNumber(r.Context(), NormalizeOrderNumber(id))
			if err != nil {
				writeError(w, err)
				return
			}
			if o == nil {
				writeError(w, ErrOrderNotFound)
				return
			}
			orderID = o.ID
		}
		d, err := svc.GetOrderDetail(r.Context(), orderID)
		if err != nil {
			writeError(w, err)
			return
		}
		if d == nil {
			writeError(w, ErrOrderNotFound)
			return
		}
		if !mayReadOrder(r, &d.Order) {
			writeJSON(w, http.StatusForbidden, map[string]any{
				"error": "this order is not readable by the caller",
				"code":  "order_forbidden",
			})
			return
		}
		writeJSON(w, http.StatusOK, d)
	})
}

// mayReadOrder is the read gate for a single order: the owning customer, or a guest who
// proves the phone on the order. Anything else is a 403, never a silent 200.
func mayReadOrder(r *http.Request, o *Order) bool {
	if o.CustomerID != "" {
		return authorization.CustomerID(r.Context()) == o.CustomerID
	}
	phone := NormalizePhone(r.URL.Query().Get("phone"))
	return phone != "" && phone == NormalizePhone(o.GuestPhone)
}

// NormalizePhone reduces a phone to its digits so ۰۹۱۲… and 0912… compare equal.
func NormalizePhone(p string) string {
	var b strings.Builder
	for _, r := range p {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r >= '۰' && r <= '۹':
			b.WriteRune('0' + (r - '۰'))
		}
	}
	out := b.String()
	if strings.HasPrefix(out, "98") && len(out) == 12 {
		out = "0" + out[2:]
	}
	return out
}

// ── GetOrderByNumber ──────────────────────────────────────────────────────────

// GetOrderByNumberHandler handles GET /api/v1/orders/by-number/{number}.
// Used by the guest order-tracking page (UJ-01): customer enters phone + order number.
func GetOrderByNumberHandler(svc *Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		num := r.PathValue("number")
		if num == "" {
			num = strings.TrimPrefix(r.URL.Path, "/api/v1/orders/by-number/")
		}
		o, err := svc.GetOrderByNumber(r.Context(), NormalizeOrderNumber(num))
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, o)
	})
}

// ── CreatePaymentIntent ───────────────────────────────────────────────────────

// CreatePaymentIntentHandler handles POST /api/v1/orders/{id}/payment-intents.
func CreatePaymentIntentHandler(svc *Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			id = strings.TrimPrefix(r.URL.Path, "/api/v1/orders/")
			// strip /payment-intents suffix
			if idx := strings.Index(id, "/"); idx >= 0 {
				id = id[:idx]
			}
		}
		pid, err := svc.CreatePaymentIntent(r.Context(), id)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"id": pid})
	})
}

// ── ConfirmPayment (webhook) ──────────────────────────────────────────────────

// ConfirmPaymentHandler handles POST /api/v1/orders/{id}/confirm-payment.
// This is the payment-provider webhook entry point (idempotency is enforced by the
// order status check: an already-paid order returns 200, not an error).
func ConfirmPaymentHandler(svc *Service, secret PaymentWebhookSecret) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The route is public, so it must prove the caller is the payment provider: the body
		// is signed with the same HMAC secret as /payments/webhook. Without this check the
		// order id alone was enough to mark an order PAID.
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "unreadable body", "code": "invalid_body"})
			return
		}
		if !VerifyWebhookSignature(secret, body, r.Header.Get("X-Provider-Signature")) {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "bad signature", "code": "invalid_signature"})
			return
		}
		id := r.PathValue("id")
		if id == "" {
			id = strings.TrimPrefix(r.URL.Path, "/api/v1/orders/")
			if idx := strings.Index(id, "/"); idx >= 0 {
				id = id[:idx]
			}
		}
		if err := svc.ConfirmPaymentForOrder(r.Context(), id); err != nil {
			// Idempotent webhook: already-paid is not an error.
			if errors.Is(err, ErrAlreadyPaid) {
				writeJSON(w, http.StatusOK, map[string]string{"status": "already_paid"})
				return
			}
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "paid"})
	})
}

// ── CancelOrder ───────────────────────────────────────────────────────────────

// CancelOrderHandler handles POST /api/v1/orders/{id}/cancel.
// Request body: {"reason": "..."}.
func CancelOrderHandler(svc *Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			id = strings.TrimPrefix(r.URL.Path, "/api/v1/orders/")
			if idx := strings.Index(id, "/"); idx >= 0 {
				id = id[:idx]
			}
		}
		var body struct {
			Reason string `json:"reason"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if err := svc.CancelOrder(r.Context(), id, body.Reason); err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
	})
}

// ── ListOrdersByCustomer (query) ──────────────────────────────────────────────
