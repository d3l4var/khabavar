package commerce

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/khabavar/khabavar/internal/modules/authorization"
)

// ── CreateOrder ───────────────────────────────────────────────────────────────

func TestHTTP_CreateOrder_Returns201(t *testing.T) {
	svc, _, _ := testService(t)
	body, _ := json.Marshal(validOrderInput())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	CreateOrderHandler(svc).ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["id"] == "" {
		t.Fatal("missing id")
	}
}

func TestHTTP_CreateOrder_RejectsBadJSON(t *testing.T) {
	svc, _, _ := testService(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", bytes.NewReader([]byte("not json")))
	rec := httptest.NewRecorder()
	CreateOrderHandler(svc).ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError && rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 4xx/5xx", rec.Code)
	}
}

func TestHTTP_CreateOrder_RejectsEmptyItems(t *testing.T) {
	svc, _, _ := testService(t)
	in := validOrderInput()
	in.Items = nil
	body, _ := json.Marshal(in)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	CreateOrderHandler(svc).ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", rec.Code)
	}
}

// ── GetOrderDetail ────────────────────────────────────────────────────────────

func TestHTTP_GetOrder_Returns200(t *testing.T) {
	svc, _, _ := testService(t)
	oid, _ := svc.CreateOrder(nil, validOrderInput())
	// The order belongs to cust-1, so the read must come from that customer — the gate is
	// part of the contract (a bare order id is not enough).
	req := httptest.NewRequest(http.MethodGet, "/api/v1/orders/"+oid, nil)
	req.SetPathValue("id", oid)
	req = req.WithContext(authorization.WithCustomerID(req.Context(), "cust-1"))
	rec := httptest.NewRecorder()
	GetOrderHandler(svc).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var d OrderDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if d.ID != oid {
		t.Fatalf("id=%s want %s", d.ID, oid)
	}
	if len(d.Items) != 1 {
		t.Fatalf("items=%d want 1", len(d.Items))
	}
}

func TestHTTP_GetOrder_NotFoundReturns404(t *testing.T) {
	svc, _, _ := testService(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/orders/no-such", nil)
	req.SetPathValue("id", "no-such")
	rec := httptest.NewRecorder()
	GetOrderHandler(svc).ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404", rec.Code)
	}
}

// ── GetOrderByNumber ──────────────────────────────────────────────────────────

func TestHTTP_GetOrderByNumber_Returns200(t *testing.T) {
	svc, _, _ := testService(t)
	svc.CreateOrder(nil, validOrderInput())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/orders/by-number/kha-ord-2026-000001", nil)
	req.SetPathValue("number", "kha-ord-2026-000001")
	rec := httptest.NewRecorder()
	GetOrderByNumberHandler(svc).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// ── ConfirmPayment (webhook idempotency) ──────────────────────────────────────

func TestHTTP_ConfirmPayment_ReturnsPaid(t *testing.T) {
	svc, _, _ := testService(t)
	oid, _ := svc.CreateOrder(nil, validOrderInput())
	svc.CreatePaymentIntent(nil, oid)

	req := signedRequest(http.MethodPost, "/api/v1/orders/"+oid+"/confirm-payment", "")
	req.SetPathValue("id", oid)
	rec := httptest.NewRecorder()
	ConfirmPaymentHandler(svc, testWebhookSecret).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp map[string]string
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["status"] != "paid" {
		t.Fatalf("status=%s want paid", resp["status"])
	}
}

func TestHTTP_ConfirmPayment_IdempotentOnSecondCall(t *testing.T) {
	svc, _, _ := testService(t)
	oid, _ := svc.CreateOrder(nil, validOrderInput())
	svc.CreatePaymentIntent(nil, oid)

	// First call: paid.
	req1 := signedRequest(http.MethodPost, "/api/v1/orders/"+oid+"/confirm-payment", "")
	req1.SetPathValue("id", oid)
	rec1 := httptest.NewRecorder()
	ConfirmPaymentHandler(svc, testWebhookSecret).ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("first: status=%d", rec1.Code)
	}

	// Second call: already_paid (idempotent, 200 not 400).
	req2 := signedRequest(http.MethodPost, "/api/v1/orders/"+oid+"/confirm-payment", "")
	req2.SetPathValue("id", oid)
	rec2 := httptest.NewRecorder()
	ConfirmPaymentHandler(svc, testWebhookSecret).ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("second: status=%d want 200 (idempotent)", rec2.Code)
	}
	var resp map[string]string
	json.Unmarshal(rec2.Body.Bytes(), &resp)
	if resp["status"] != "already_paid" {
		t.Fatalf("second: status=%s want already_paid", resp["status"])
	}
}

// ── CancelOrder ───────────────────────────────────────────────────────────────

func TestHTTP_CancelOrder_ReturnsCancelled(t *testing.T) {
	svc, _, _ := testService(t)
	oid, _ := svc.CreateOrder(nil, validOrderInput())
	body, _ := json.Marshal(map[string]string{"reason": "test"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/orders/"+oid+"/cancel", bytes.NewReader(body))
	req.SetPathValue("id", oid)
	rec := httptest.NewRecorder()
	CancelOrderHandler(svc).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHTTP_CancelOrder_PaidOrderReturns400(t *testing.T) {
	svc, _, _ := testService(t)
	oid, _ := svc.CreateOrder(nil, validOrderInput())
	svc.CreatePaymentIntent(nil, oid)
	svc.ConfirmPaymentForOrder(nil, oid)

	body, _ := json.Marshal(map[string]string{"reason": "test"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/orders/"+oid+"/cancel", bytes.NewReader(body))
	req.SetPathValue("id", oid)
	rec := httptest.NewRecorder()
	CancelOrderHandler(svc).ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", rec.Code)
	}
}

// ── CreatePaymentIntent ───────────────────────────────────────────────────────

func TestHTTP_CreatePaymentIntent_Returns201(t *testing.T) {
	svc, _, _ := testService(t)
	oid, _ := svc.CreateOrder(nil, validOrderInput())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/orders/"+oid+"/payment-intents", nil)
	req.SetPathValue("id", oid)
	rec := httptest.NewRecorder()
	CreatePaymentIntentHandler(svc).ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHTTP_CreatePaymentIntent_DuplicateReturns400(t *testing.T) {
	svc, _, _ := testService(t)
	oid, _ := svc.CreateOrder(nil, validOrderInput())
	svc.CreatePaymentIntent(nil, oid)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/orders/"+oid+"/payment-intents", nil)
	req.SetPathValue("id", oid)
	rec := httptest.NewRecorder()
	CreatePaymentIntentHandler(svc).ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", rec.Code)
	}
}

// testWebhookSecret is the secret the payment tests sign with.
const testWebhookSecret PaymentWebhookSecret = "test-webhook-secret"

// signedRequest builds a request whose X-Provider-Signature matches its body.
func signedRequest(method, target, body string) *http.Request {
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	mac := hmac.New(sha256.New, []byte(testWebhookSecret))
	mac.Write([]byte(body))
	r.Header.Set("X-Provider-Signature", hex.EncodeToString(mac.Sum(nil)))
	return r
}
