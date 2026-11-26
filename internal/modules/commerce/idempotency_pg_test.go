package commerce

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestPG_IdempotencyMiddleware drives the middleware that guards every money operation:
// a missing key is refused, a replay returns the first response verbatim, the same key
// with a different body is a conflict, and a failure stays retryable.
func TestPG_IdempotencyMiddleware(t *testing.T) {
	pool, _, _ := setupCommerce(t)
	store := NewDBStore(pool) // DBStore implements IdempotencyStore
	ctx := context.Background()

	var calls int
	handler := Idempotent(store, "test-scope", func(w http.ResponseWriter, r *http.Request) {
		calls++
		writeJSON(w, http.StatusCreated, map[string]any{"call": calls})
	})

	do := func(key, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(body))
		if key != "" {
			r.Header.Set("Idempotency-Key", key)
		}
		rec := httptest.NewRecorder()
		handler(rec, r)
		return rec
	}

	// No key: refused before the handler runs.
	if rec := do("", `{"a":1}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("missing key = %d, want 400", rec.Code)
	}
	if calls != 0 {
		t.Fatalf("handler ran without a key (%d calls)", calls)
	}

	key := "idem-" + uuid.NewString()
	first := do(key, `{"a":1}`)
	if first.Code != http.StatusCreated {
		t.Fatalf("first = %d: %s", first.Code, first.Body.String())
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}

	// The replay must return the first body and must not run the handler again.
	second := do(key, `{"a":1}`)
	if second.Code != http.StatusCreated {
		t.Fatalf("replay = %d: %s", second.Code, second.Body.String())
	}
	// The stored body is a jsonb column, so Postgres normalises whitespace: the replay is
	// semantically identical but not byte-identical. Compare the decoded value, and do not
	// claim byte fidelity the schema cannot give.
	var firstJSON, secondJSON map[string]any
	_ = json.Unmarshal(first.Body.Bytes(), &firstJSON)
	_ = json.Unmarshal(second.Body.Bytes(), &secondJSON)
	if len(firstJSON) != len(secondJSON) || firstJSON["call"] != secondJSON["call"] {
		t.Fatalf("replay body = %s, want the same value as %s", second.Body.String(), first.Body.String())
	}
	if second.Header().Get("Idempotent-Replay") != "true" {
		t.Fatal("a replay must be marked with Idempotent-Replay")
	}
	if calls != 1 {
		t.Fatalf("the handler ran again on a replay (calls = %d)", calls)
	}

	// Same key, different body: a conflict, not a silent second operation.
	if rec := do(key, `{"a":2}`); rec.Code != http.StatusConflict {
		t.Fatalf("different body = %d, want 409", rec.Code)
	}

	// A failing handler must leave the key retryable.
	failKey := "idem-" + uuid.NewString()
	failing := Idempotent(store, "test-scope", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "nope"})
	})
	r := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"b":1}`))
	r.Header.Set("Idempotency-Key", failKey)
	rec := httptest.NewRecorder()
	failing(rec, r)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("failing handler = %d", rec.Code)
	}
	// The same key must be claimable again (the first attempt stored no response).
	owned, _, err := store.Claim(ctx, failKey, "test-scope", HashRequest([]byte(`{"b":1}`)), 24*time.Hour)
	if err != nil {
		t.Fatalf("Claim after failure: %v", err)
	}
	if owned {
		t.Fatal("a failed attempt must not keep the key owned")
	}
}

// signForTest mirrors what a gateway does: HMAC-SHA256 over the raw body.
func signForTest(secret PaymentWebhookSecret, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// TestPG_PaymentHandlers covers the two payment routes end to end: the webhook's signature
// gate and confirmation, and the browser return redirect.
func TestPG_PaymentHandlers(t *testing.T) {
	pool, svc, store := setupCommerce(t)
	ctx := context.Background()
	productID, variantID := seedProductVariant(t, ctx, pool, "SKU-WH2-1")
	customerID := seedCustomerRow(t, ctx, pool)

	orderID, err := svc.CreateOrder(ctx, OrderInput{
		CustomerID: customerID, Currency: "IRR",
		Items: []OrderItemInput{{
			ProductID: productID, VariantID: variantID, SKUSnapshot: "SKU-WH2-1",
			TitleSnapshot: "x", Quantity: 1, UnitPrice: 5000, SupplyType: "OWNED_INVENTORY",
		}},
		ShippingAddr: json.RawMessage(`{"city":"تهران"}`),
	})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	intentID, err := svc.CreatePaymentIntent(ctx, orderID)
	if err != nil {
		t.Fatalf("CreatePaymentIntent: %v", err)
	}
	ref := "REF-WH2-" + orderID[:8]
	if _, err := store.CreateAttempt(ctx, PaymentAttempt{
		PaymentIntentID: intentID, Provider: "mock", ProviderReference: ref,
		Amount: 5000, Status: "SUCCEEDED", IdempotencyKey: "idem-" + orderID[:8],
		CompletedAt: ptrTime(time.Now()),
	}); err != nil {
		t.Fatalf("CreateAttempt: %v", err)
	}

	secret := PaymentWebhookSecret("whsec-test")
	h := PaymentWebhookHandler(svc, secret)
	body := `{"provider":"mock","provider_reference":"` + ref + `","amount":5000,"status":"OK"}`

	post := func(sig string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/payments/webhook", strings.NewReader(body))
		if sig != "" {
			r.Header.Set("X-Provider-Signature", sig)
		}
		rec := httptest.NewRecorder()
		h(rec, r)
		return rec
	}

	if rec := post(""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("no signature = %d, want 401", rec.Code)
	}
	if rec := post("deadbeef"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad signature = %d, want 401", rec.Code)
	}
	sig := signForTest(secret, []byte(body))
	if rec := post(sig); rec.Code != http.StatusOK {
		t.Fatalf("good signature = %d: %s", rec.Code, rec.Body.String())
	}
	o, _ := store.GetOrder(ctx, orderID)
	if o.Status != OrderPaid {
		t.Fatalf("order status = %q, want PAID", o.Status)
	}
	// A provider retry must be a 2xx, not a 500.
	rec := post(sig)
	if rec.Code != http.StatusOK {
		t.Fatalf("replay = %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "already_confirmed") {
		t.Fatalf("replay body = %s", rec.Body.String())
	}

	// The return route only redirects.
	rh := PaymentReturnHandler("https://shop.khabavar.ir")
	r := httptest.NewRequest(http.MethodGet, "/api/v1/payments/return?order_number=KHA-1", nil)
	rr := httptest.NewRecorder()
	rh(rr, r)
	if rr.Code != http.StatusFound || !strings.HasSuffix(rr.Header().Get("Location"), "/order/KHA-1") {
		t.Fatalf("return = %d %q", rr.Code, rr.Header().Get("Location"))
	}
	// A missing order_number is a 400, not a redirect to a broken page.
	r = httptest.NewRequest(http.MethodGet, "/api/v1/payments/return", nil)
	rr = httptest.NewRecorder()
	rh(rr, r)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("return without order_number = %d, want 400", rr.Code)
	}
}
