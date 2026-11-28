package commerce

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// fakeZarinpal is a stand-in for Zarinpal's v4 REST API: request.json opens a transaction,
// verify.json settles it. The response shape is the real one — a numeric code where 100 means
// success and 101 means already verified.
type fakeZarinpal struct {
	requestCode  int
	verifyCode   int
	lastAmount   int64
	lastCallback string
	authority    string
}

func (f *fakeZarinpal) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var body struct {
		MerchantID  string `json:"merchant_id"`
		Amount      int64  `json:"amount"`
		CallbackURL string `json:"callback_url"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	f.lastAmount = body.Amount
	f.lastCallback = body.CallbackURL

	switch {
	case strings.HasSuffix(r.URL.Path, "/request.json"):
		if f.requestCode != 100 {
			_, _ = w.Write([]byte(`{"data":[],"errors":{"code":-9,"message":"validation error"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"code":100,"message":"Success","authority":"` + f.authority + `"},"errors":[]}`))
	case strings.HasSuffix(r.URL.Path, "/verify.json"):
		code := f.verifyCode
		if code == 0 {
			code = 100
		}
		if code == 100 || code == 101 {
			_, _ = w.Write([]byte(`{"data":{"code":` + strconv.Itoa(code) + `,"ref_id":201,"message":"Verified"},"errors":[]}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[],"errors":{"code":-51,"message":"failed"}}`))
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func newZarinpal(t *testing.T, fake *fakeZarinpal) ZarinpalGateway {
	t.Helper()
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	return ZarinpalGateway{
		BaseURL:     srv.URL,
		StartURL:    "https://gateway.test/StartPay/",
		MerchantID:  "merchant-1",
		CallbackURL: "https://shop.test/pay/callback",
	}
}

// TestZarinpal_StartReturnsRedirect covers the call that was missing entirely: the customer must
// be sent somewhere to pay.
func TestZarinpal_StartReturnsRedirect(t *testing.T) {
	fake := &fakeZarinpal{requestCode: 100, authority: "A0000000000000000000000000000012345"}
	gw := newZarinpal(t, fake)

	res, err := gw.Start(context.Background(), StartPayment{
		OrderID: "ord-1", Amount: 250000, Description: "سفارش KHA-1",
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if res.Authority != fake.authority {
		t.Fatalf("authority=%q want %q", res.Authority, fake.authority)
	}
	if res.RedirectURL != "https://gateway.test/StartPay/"+fake.authority {
		t.Fatalf("redirect=%q", res.RedirectURL)
	}
	// The amount must reach the gateway as an integer rial value, not a formatted string.
	if fake.lastAmount != 250000 {
		t.Fatalf("gateway saw amount %d, want 250000", fake.lastAmount)
	}
	if fake.lastCallback != "https://shop.test/pay/callback" {
		t.Fatalf("callback=%q", fake.lastCallback)
	}
}

// TestZarinpal_StartRejectionIsSentinel: a business rejection must be distinguishable from a
// transport failure so the handler can answer 409 instead of 500.
func TestZarinpal_StartRejectionIsSentinel(t *testing.T) {
	gw := newZarinpal(t, &fakeZarinpal{requestCode: -9})
	_, err := gw.Start(context.Background(), StartPayment{OrderID: "o", Amount: 1000})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "rejected") {
		t.Fatalf("err=%v, want a rejection", err)
	}
}

// TestZarinpal_VerifyTreats101AsPaid pins the rule that protects the customer: 101 means the
// transaction was already verified. Treating it as a failure would charge someone twice after
// they refreshed the callback page.
func TestZarinpal_VerifyTreats101AsPaid(t *testing.T) {
	for _, code := range []int{100, 101} {
		gw := newZarinpal(t, &fakeZarinpal{verifyCode: code})
		res, err := gw.Verify(context.Background(), VerifyPayment{Authority: "A1", Amount: 5000})
		if err != nil {
			t.Fatalf("code %d: Verify: %v", code, err)
		}
		if !res.Paid {
			t.Fatalf("code %d: Paid=false, want true", code)
		}
		if res.Reference != "201" {
			t.Fatalf("code %d: reference=%q want 201", code, res.Reference)
		}
	}
}

// TestZarinpal_VerifyRejectsOtherCodes: anything else is not a payment.
func TestZarinpal_VerifyRejectsOtherCodes(t *testing.T) {
	gw := newZarinpal(t, &fakeZarinpal{verifyCode: -51})
	res, err := gw.Verify(context.Background(), VerifyPayment{Authority: "A1", Amount: 5000})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if res.Paid {
		t.Fatal("Paid=true for a failed verification")
	}
}

// TestMockGateway_CompletesPurchase: the default adapter must never block a checkout.
func TestMockGateway_CompletesPurchase(t *testing.T) {
	gw := MockGateway{}
	res, err := gw.Start(context.Background(), StartPayment{OrderID: "ord-9", Amount: 1000})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if res.RedirectURL == "" {
		t.Fatal("mock gateway returned no redirect")
	}
	v, err := gw.Verify(context.Background(), VerifyPayment{Authority: res.Authority, Amount: 1000})
	if err != nil || !v.Paid {
		t.Fatalf("Verify = (%v,%v), want paid", v, err)
	}
}
