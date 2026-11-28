package commerce

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

// PaymentCallbackHandler serves GET /api/v1/payments/callback — the page the gateway sends the
// customer's browser back to.
//
// Why it exists: the only confirmation path was a server-to-server webhook signed with an HMAC.
// Real Iranian gateways (Zarinpal, IDPay, Zibal) return the customer's BROWSER to a callback URL
// with ?Authority=...&Status=OK and no signature at all. Without this route a real payment was
// never claimed: the money left the customer's account and the order stayed PENDING_PAYMENT.
//
// The query string is never trusted. Status=OK only means "the gateway says so"; the amount and
// the settlement are confirmed with Verify before anything is marked paid.
func PaymentCallbackHandler(svc *Service, redirectOK, redirectFail string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		authority := firstNonEmpty(q.Get("Authority"), q.Get("authority"))
		status := firstNonEmpty(q.Get("Status"), q.Get("status"))

		if authority == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error": "missing authority", "code": "invalid_callback",
			})
			return
		}
		// A cancelled payment is not an error worth a 500; the customer simply did not pay.
		if status != "" && !strings.EqualFold(status, "OK") {
			writeJSON(w, http.StatusConflict, map[string]any{
				"error": "payment not completed", "code": "payment_cancelled",
			})
			return
		}

		intentID, orderID, err := svc.FindIntentByProviderReference(r.Context(), authority)
		if err != nil || orderID == "" {
			writeJSON(w, http.StatusNotFound, map[string]any{
				"error": "unknown payment", "code": "unknown_payment",
			})
			return
		}

		paid, err := svc.VerifyWithGateway(r.Context(), authority, orderID)
		if err != nil {
			// The gateway is unreachable or answered something we cannot read. Do not mark the
			// order paid on a guess; the webhook or a reconcile run will settle it.
			writeJSON(w, http.StatusBadGateway, map[string]any{
				"error": "could not verify payment", "code": "verify_failed",
			})
			return
		}
		if !paid {
			writeJSON(w, http.StatusConflict, map[string]any{
				"error": "payment not verified", "code": "payment_not_verified",
			})
			return
		}

		if err := svc.ConfirmPayment(r.Context(), intentID); err != nil && !errors.Is(err, ErrAlreadyPaid) {
			writeJSON(w, http.StatusInternalServerError, map[string]any{
				"error": "could not confirm order", "code": "confirm_failed",
			})
			return
		}

		// A browser landed here, so send it somewhere a human can read.
		if redirectOK != "" {
			http.Redirect(w, r, redirectOK, http.StatusSeeOther)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "paid", "order_id": orderID})
	})
}

// VerifyWithGateway asks the configured gateway whether the authority was actually paid, and compares
// the amount with the order total. It returns false when the gateway says no.
func (s *Service) VerifyWithGateway(ctx context.Context, authority, orderID string) (bool, error) {
	if s.gateway == nil {
		return false, errors.New("commerce: no payment gateway configured")
	}
	o, err := s.store.GetOrder(ctx, orderID)
	if err != nil {
		return false, err
	}
	if o == nil {
		return false, ErrOrderNotFound
	}
	res, err := s.gateway.Verify(ctx, VerifyPayment{Authority: authority, Amount: o.GrandTotal})
	if err != nil {
		return false, err
	}
	if !res.Paid {
		return false, nil
	}
	// The gateway echoes an amount; a mismatch means the customer paid less than the order.
	if res.Amount != 0 && res.Amount != o.GrandTotal {
		return false, nil
	}
	return true, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
