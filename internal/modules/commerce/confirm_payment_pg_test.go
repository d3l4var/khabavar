package commerce

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestPG_ConfirmPaymentByIntent: ConfirmPayment used to be a stub that always returned an
// error ("use ConfirmPaymentForOrder"). It now resolves the intent to its order and
// delegates, so there is one confirmation path instead of two behaviours.
func TestPG_ConfirmPaymentByIntent(t *testing.T) {
	pool, svc, store := setupCommerce(t)
	ctx := context.Background()
	productID, variantID := seedProductVariant(t, ctx, pool, "SKU-CPI-1")
	customerID := seedCustomerRow(t, ctx, pool)

	orderID, err := svc.CreateOrder(ctx, OrderInput{
		CustomerID: customerID, Currency: "IRR",
		Items: []OrderItemInput{{
			ProductID: productID, VariantID: variantID, SKUSnapshot: "SKU-CPI-1",
			TitleSnapshot: "x", Quantity: 1, UnitPrice: 7000, SupplyType: "OWNED_INVENTORY",
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
	if _, err := store.CreateAttempt(ctx, PaymentAttempt{
		PaymentIntentID: intentID, Provider: "mock", ProviderReference: "REF-CPI-" + orderID[:8],
		Amount: 7000, Status: "SUCCEEDED", IdempotencyKey: "idem-" + orderID[:8],
		CompletedAt: ptrTime(time.Now()),
	}); err != nil {
		t.Fatalf("CreateAttempt: %v", err)
	}

	// The store lookup directly, so the SQL method has a named PG case of its own.
	if got, err := store.OrderIDForIntent(ctx, intentID); err != nil || got != orderID {
		t.Fatalf("OrderIDForIntent = %q, %v; want %q", got, err, orderID)
	}
	if got, err := store.OrderIDForIntent(ctx, "00000000-0000-0000-0000-000000000000"); err != nil || got != "" {
		t.Fatalf("OrderIDForIntent(unknown) = %q, %v; want empty", got, err)
	}

	if err := svc.ConfirmPayment(ctx, intentID); err != nil {
		t.Fatalf("ConfirmPayment: %v", err)
	}
	o, _ := store.GetOrder(ctx, orderID)
	if o.Status != OrderPaid {
		t.Fatalf("status = %q, want PAID", o.Status)
	}

	// An unknown intent is a domain error, not a panic or a 500.
	if err := svc.ConfirmPayment(ctx, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, ErrPaymentIntentNotFound) {
		t.Fatalf("unknown intent = %v, want ErrPaymentIntentNotFound", err)
	}
	if err := svc.ConfirmPayment(ctx, ""); !errors.Is(err, ErrPaymentIntentNotFound) {
		t.Fatalf("empty intent = %v, want ErrPaymentIntentNotFound", err)
	}
}

// TestPG_ListOrdersHandler covers the panel order list, including the paging guards.
func TestPG_ListOrdersHandler(t *testing.T) {
	pool, svc, _ := setupCommerce(t)
	ctx := context.Background()
	productID, variantID := seedProductVariant(t, ctx, pool, "SKU-LOH-1")
	customerID := seedCustomerRow(t, ctx, pool)

	for i := 0; i < 2; i++ {
		if _, err := svc.CreateOrder(ctx, OrderInput{
			CustomerID: customerID, Currency: "IRR",
			Items: []OrderItemInput{{
				ProductID: productID, VariantID: variantID, SKUSnapshot: "SKU-LOH-1",
				TitleSnapshot: "x", Quantity: 1, UnitPrice: 1000, SupplyType: "OWNED_INVENTORY",
			}},
			ShippingAddr: json.RawMessage(`{"city":"تهران"}`),
		}); err != nil {
			t.Fatalf("CreateOrder: %v", err)
		}
	}

	h := ListOrdersHandler(NewOrderListStore(pool))
	// Nonsense paging must fall back to sane defaults instead of erroring.
	r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/orders?page=0&page_size=9999", nil)
	rec := httptest.NewRecorder()
	h(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("list = %d: %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Data     []OrderSummary `json:"data"`
		Total    int            `json:"total"`
		Page     int            `json:"page"`
		PageSize int            `json:"page_size"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Page != 1 || out.PageSize != 20 {
		t.Fatalf("paging = %d/%d, want 1/20", out.Page, out.PageSize)
	}
	if out.Total < 2 || len(out.Data) < 2 {
		t.Fatalf("orders = %d total %d, want at least 2", len(out.Data), out.Total)
	}

	// A status filter that matches nothing is an empty list, not an error.
	r = httptest.NewRequest(http.MethodGet, "/api/v1/admin/orders?status=CANCELLED", nil)
	rec = httptest.NewRecorder()
	h(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("filtered list = %d: %s", rec.Code, rec.Body.String())
	}
}
