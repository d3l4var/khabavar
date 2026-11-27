package commerce

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/khabavar/khabavar/internal/modules/authorization"
)

// failingStore embeds the real store so only the methods under test need overriding: a
// store failure must surface as a 5xx with a stable body, never as a 200 with empty data.
type failingStore struct {
	*DBStore
	err error
}

func (f failingStore) ListRefunds(context.Context, string) ([]Refund, error) { return nil, f.err }
func (f failingStore) GetOrderDetail(context.Context, string) (*OrderDetail, error) {
	return nil, f.err
}
func (f failingStore) SumSucceededAttempts(context.Context, string) (int64, error) { return 0, f.err }
func (f failingStore) GetOrder(context.Context, string) (*Order, error)            { return nil, f.err }
func (f failingStore) GetActivePaymentIntent(context.Context, string) (*PaymentIntent, error) {
	return nil, f.err
}

// TestPG_CommerceStoreFailuresAreServerErrors drives the handler error branches that a
// healthy database never reaches.
func TestPG_CommerceStoreFailuresAreServerErrors(t *testing.T) {
	_, svc, store := setupCommerce(t)
	svc.SetCartStore(store)
	failing := failingStore{DBStore: store, err: errors.New("db down")}
	broken := NewService(failing, nil)
	broken.SetRefundStore(failing) // wired to the failing store, so the error comes from it

	call := func(h http.Handler, method, body, id string) *httptest.ResponseRecorder {
		var r *http.Request
		if body == "" {
			r = httptest.NewRequest(method, "/x", nil)
		} else {
			r = httptest.NewRequest(method, "/x", strings.NewReader(body))
		}
		r = r.WithContext(authorization.WithCustomerID(r.Context(), "00000000-0000-0000-0000-000000000001"))
		if id != "" {
			r.SetPathValue("id", id)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		return rec
	}

	missing := "00000000-0000-0000-0000-000000000000"
	cases := map[string]struct {
		h    http.Handler
		body string
	}{
		"refund list":   {ListRefundsHandler(broken), ""},
		"reconcile":     {ReconcileTransactionHandler(broken), ""},
		"order detail":  {GetOrderHandler(broken), ""},
		"create refund": {CreateRefundHandler(broken), `{"amount":1000,"reason":"x"}`},
	}
	for name, c := range cases {
		rec := call(c.h, http.MethodPost, c.body, missing)
		if rec.Code < 500 {
			t.Fatalf("%s with a failing store = %d, want 5xx: %s", name, rec.Code, rec.Body.String())
		}
	}

	// The healthy service still answers 4xx (not 5xx) for an unknown order.
	if rec := call(ListRefundsHandler(svc), http.MethodGet, "", missing); rec.Code >= 500 {
		t.Fatalf("refund list for an unknown order = %d, want a 4xx", rec.Code)
	}
}
