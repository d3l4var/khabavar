package fulfillment

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestCreateOrderFulfillmentRejectsContradictingBody pins the guard: the path id is
// authoritative, so a body order_id that disagrees is a 400, not a silent fulfilment of
// the wrong order.
func TestCreateOrderFulfillmentRejectsContradictingBody(t *testing.T) {
	h := CreateOrderFulfillmentHandler(nil) // nil service: the guard must fire before any call
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/orders/ord-1/fulfillments",
		strings.NewReader(`{"order_id":"ord-2","source_type":"OWNED_INVENTORY"}`))
	req.SetPathValue("id", "ord-1")
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body contradicts the path)", rec.Code)
	}
}
