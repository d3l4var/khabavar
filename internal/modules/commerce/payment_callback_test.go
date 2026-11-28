package commerce

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// stubGateway answers Verify with a fixed verdict, so the handler's decisions can be tested
// without an HTTP provider.
type stubGateway struct {
	paid   bool
	amount int64
	err    error
	calls  int
}

func (s *stubGateway) Name() string { return "stub" }
func (s *stubGateway) Start(context.Context, StartPayment) (StartResult, error) {
	return StartResult{Authority: "AUTH-1", RedirectURL: "https://gw.test/AUTH-1"}, nil
}
func (s *stubGateway) Verify(context.Context, VerifyPayment) (VerifyResult, error) {
	s.calls++
	if s.err != nil {
		return VerifyResult{}, s.err
	}
	return VerifyResult{Paid: s.paid, Amount: s.amount, Reference: "REF-1"}, nil
}

func callbackService(t *testing.T, gw PaymentGateway) *Service {
	t.Helper()
	store := newMemStore()
	store.intentByRef = map[string]string{"AUTH-1": "intent-1"}
	svc := NewService(store, nil)
	svc.SetPaymentGateway(gw)
	// One order in PENDING_PAYMENT with a known total, and an intent pointing at it.
	store.orders["ord-1"] = &Order{ID: "ord-1", OrderNumber: "KHA-1", Status: OrderPendingPayment, GrandTotal: 25000, Currency: "IRR"}
	store.intents["ord-1"] = []*PaymentIntent{{ID: "intent-1", OrderID: "ord-1", Amount: 25000, Status: IntentRequiresPayment}}
	return svc
}

// FindIntentByProviderReference lets the callback resolve an authority to its intent, the same
// way DBStore does from payment_attempts.
func (m *memStore) FindIntentByProviderReference(_ context.Context, ref string) (string, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	intentID, ok := m.intentByRef[ref]
	if !ok {
		return "", "", nil
	}
	for orderID, intents := range m.intents {
		for _, it := range intents {
			if it.ID == intentID {
				return intentID, orderID, nil
			}
		}
	}
	return "", "", nil
}

// TestPaymentCallback_TrustsVerifyNotQueryString is the whole point of the route: the gateway
// says Status=OK, but the money is only real if Verify agrees.
func TestPaymentCallback_TrustsVerifyNotQueryString(t *testing.T) {
	gw := &stubGateway{paid: false}
	svc := callbackService(t, gw)
	rec := httptest.NewRecorder()
	PaymentCallbackHandler(svc, "", "").ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		"/api/v1/payments/callback?Authority=AUTH-1&Status=OK", nil))

	if gw.calls != 1 {
		t.Fatalf("Verify called %d times, want 1", gw.calls)
	}
	if rec.Code != http.StatusConflict {
		t.Fatalf("status=%d want 409; body=%s", rec.Code, rec.Body.String())
	}
}

// TestPaymentCallback_VerifiesThenConfirms: a verified payment marks the order paid.
func TestPaymentCallback_VerifiesThenConfirms(t *testing.T) {
	gw := &stubGateway{paid: true, amount: 25000}
	svc := callbackService(t, gw)
	rec := httptest.NewRecorder()
	PaymentCallbackHandler(svc, "", "").ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		"/api/v1/payments/callback?Authority=AUTH-1&Status=OK", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", rec.Code, rec.Body.String())
	}
}

// TestPaymentCallback_AmountMismatchIsNotPaid: a gateway that settles less than the order total
// must not mark it paid.
func TestPaymentCallback_AmountMismatchIsNotPaid(t *testing.T) {
	gw := &stubGateway{paid: true, amount: 1000}
	svc := callbackService(t, gw)
	rec := httptest.NewRecorder()
	PaymentCallbackHandler(svc, "", "").ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		"/api/v1/payments/callback?Authority=AUTH-1&Status=OK", nil))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status=%d want 409 for a short payment", rec.Code)
	}
}

// TestPaymentCallback_CancelledAndUnknown covers the two non-error answers a browser can hit.
func TestPaymentCallback_CancelledAndUnknown(t *testing.T) {
	svc := callbackService(t, &stubGateway{paid: true, amount: 25000})

	rec := httptest.NewRecorder()
	PaymentCallbackHandler(svc, "", "").ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		"/api/v1/payments/callback?Authority=AUTH-1&Status=NOK", nil))
	if rec.Code != http.StatusConflict {
		t.Fatalf("cancelled: status=%d want 409", rec.Code)
	}

	rec = httptest.NewRecorder()
	PaymentCallbackHandler(svc, "", "").ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		"/api/v1/payments/callback?Authority=UNKNOWN", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown: status=%d want 404", rec.Code)
	}
}

// TestConfirmPayment_SecondSettleIsNotAnError pins the race a gateway produces for free: it
// retries its callback. The second settle used to answer ErrIntentNotSucceeded, which the
// callback handler turned into a 500 for a payment that had already succeeded.
func TestConfirmPayment_SecondSettleIsNotAnError(t *testing.T) {
	store := newMemStore()
	svc := NewService(store, nil)
	store.orders["ord-2"] = &Order{ID: "ord-2", OrderNumber: "KHA-2", Status: OrderPendingPayment, GrandTotal: 25000, Currency: "IRR"}
	store.intents["ord-2"] = []*PaymentIntent{{ID: "intent-2", OrderID: "ord-2", Amount: 25000, Status: IntentRequiresPayment}}

	if err := svc.ConfirmPayment(context.Background(), "intent-2"); err != nil {
		t.Fatalf("first confirm: %v", err)
	}
	err := svc.ConfirmPayment(context.Background(), "intent-2")
	if !errors.Is(err, ErrAlreadyPaid) {
		t.Fatalf("second confirm = %v, want ErrAlreadyPaid", err)
	}
}
