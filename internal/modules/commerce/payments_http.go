package commerce

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// PaymentWebhookSecret is the HMAC key the gateway signs notifications with. It comes
// from config; an empty key disables the webhook (fail closed, never fail open).
type PaymentWebhookSecret string

// VerifyWebhookSignature compares the header against HMAC-SHA256(raw body) in constant
// time. A raw-body signature is the only safe scheme: re-encoding the JSON would change
// the bytes the provider signed.
func VerifyWebhookSignature(secret PaymentWebhookSecret, body []byte, header string) bool {
	if secret == "" || header == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	want := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(want), []byte(strings.ToLower(strings.TrimSpace(header))))
}

// webhookPayload is the contract's notification body.
type webhookPayload struct {
	Provider          string `json:"provider"`
	ProviderReference string `json:"provider_reference"`
	Amount            int64  `json:"amount"`
	Status            string `json:"status"`
}

// PaymentWebhookHandler serves POST /api/v1/payments/webhook.
//
// Trust model: the raw notification is never trusted for the outcome — it only tells us
// which transaction to look at. Confirmation goes through ConfirmPayment, which is the
// same path the panel and tests use, and a duplicate notification is a no-op because
// the intent is already SUCCEEDED.
func PaymentWebhookHandler(svc *Service, secret PaymentWebhookSecret) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "unreadable body", "code": "invalid_body"})
			return
		}
		if !VerifyWebhookSignature(secret, body, r.Header.Get("X-Provider-Signature")) {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "bad signature", "code": "invalid_signature"})
			return
		}
		var p webhookPayload
		if err := json.Unmarshal(body, &p); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json", "code": "invalid_body"})
			return
		}
		if p.ProviderReference == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "provider_reference is required", "code": "invalid_body"})
			return
		}
		_, orderID, err := svc.FindIntentByProviderReference(r.Context(), p.ProviderReference)
		if err != nil || orderID == "" {
			// Unknown reference: answer 202 so the provider stops retrying, but do not
			// pretend anything was confirmed.
			writeJSON(w, http.StatusAccepted, map[string]any{"status": "ignored", "reason": "unknown reference"})
			return
		}
		// A gateway retries until it gets a 2xx, so an already-paid order is a success,
		// not an error: answering 500 here would make the provider retry forever.
		if order, err := svc.GetOrderDetail(r.Context(), orderID); err == nil && order != nil {
			if order.Status == OrderPaid || order.Status == OrderCompleted || order.Status == OrderInFulfillment {
				writeJSON(w, http.StatusOK, map[string]any{"status": "already_confirmed"})
				return
			}
		}

		// Confirmation goes through the order, which is the path that also writes the
		// payment attempt and the outbox event in one transaction.
		if err := svc.ConfirmPaymentForOrder(r.Context(), orderID); err != nil {
			if errors.Is(err, ErrOrderNotFound) {
				writeJSON(w, http.StatusAccepted, map[string]any{"status": "ignored"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal error", "code": "internal_error"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "confirmed"})
	}
}

// PaymentReturnHandler serves GET /api/v1/payments/return: the browser coming back from
// the gateway. It never confirms anything on its own — it only sends the customer to the
// order page, where the status reflects whatever the webhook confirmed.
func PaymentReturnHandler(storefrontBase string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orderNumber := r.URL.Query().Get("order_number")
		if orderNumber == "" {
			http.Error(w, "order_number is required", http.StatusBadRequest)
			return
		}
		// The base comes from config so the redirect cannot be pointed anywhere by the
		// caller (an open redirect is a phishing primitive).
		target := strings.TrimRight(storefrontBase, "/") + "/order/" + url.PathEscape(orderNumber)
		http.Redirect(w, r, target, http.StatusFound)
	}
}
