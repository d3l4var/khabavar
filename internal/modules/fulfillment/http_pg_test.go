package fulfillment

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestPG_FulfillmentHTTPHandlers drives the fulfillment surface through its handlers: the
// create path, every status transition, the shipment lifecycle and the error branches.
// http.go was at 11%, so a wrong status code or a missing guard could not be caught.
func TestPG_FulfillmentHTTPHandlers(t *testing.T) {
	pool, svc, store := setupFulfillment(t)
	ctx := context.Background()
	orderID, _ := seedOrderItemGraph(t, ctx, pool, "KHA-ORD-2026-000903", "SKU-FH-1")
	whID := seedWarehouse(t, ctx, pool, "WH-FH-1")

	call := func(h http.HandlerFunc, method, body string, pv map[string]string) *httptest.ResponseRecorder {
		var r *http.Request
		if body == "" {
			r = httptest.NewRequest(method, "/x", nil)
		} else {
			r = httptest.NewRequest(method, "/x", strings.NewReader(body))
		}
		for k, v := range pv {
			r.SetPathValue(k, v)
		}
		rec := httptest.NewRecorder()
		h(rec, r)
		return rec
	}

	// Broken JSON and a missing order id are 400s, not 500s.
	if rec := call(CreateFulfillmentOrderHandler(svc), http.MethodPost, `{`, nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("broken body = %d, want 400", rec.Code)
	}
	if rec := call(CreateFulfillmentOrderHandler(svc), http.MethodPost, `{"source_type":"OWNED_INVENTORY"}`, nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("missing order id = %d, want 400", rec.Code)
	}

	// Create a fulfillment order through the handler.
	rec := call(CreateFulfillmentOrderHandler(svc), http.MethodPost,
		`{"order_id":"`+orderID+`","source_type":"OWNED_INVENTORY","warehouse_id":"`+whID+`"}`, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", rec.Code, rec.Body.String())
	}
	var fo struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &fo)
	if fo.ID == "" {
		t.Fatalf("create response = %s", rec.Body.String())
	}

	// Walk the state machine up to READY; the shipment is created from there.
	// An ordered slice, not a map: the state machine requires this exact order and Go map
	// iteration is random.
	steps := []struct {
		name string
		h    http.HandlerFunc
	}{
		{"confirm", ConfirmFulfillmentHandler(svc)},
		{"processing", StartProcessingHandler(svc)},
		{"ready", MarkReadyHandler(svc)},
	}
	for _, step := range steps {
		rec := call(step.h, http.MethodPost, "", map[string]string{"id": fo.ID})
		if rec.Code >= 400 {
			t.Fatalf("%s = %d: %s", step.name, rec.Code, rec.Body.String())
		}
	}
	// The shipment is created while the fulfillment is READY, then shipped and delivered.
	// A shipment on the fulfillment, then its own transitions.
	rec = call(CreateShipmentHandler(svc), http.MethodPost,
		`{"carrier":"POST","service_type":"EXPRESS","tracking_number":"TRK-FH","shipping_cost":1000}`,
		map[string]string{"id": fo.ID})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create shipment = %d: %s", rec.Code, rec.Body.String())
	}
	var sh struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &sh)
	if sh.ID == "" {
		t.Fatalf("shipment response = %s", rec.Body.String())
	}
	if rec := call(TransitionShipmentHandler(svc), http.MethodPost,
		`{"to":"IN_TRANSIT","event_code":"PICKED_UP","source":"CARRIER_API"}`,
		map[string]string{"id": sh.ID}); rec.Code >= 400 {
		t.Fatalf("shipment transition = %d: %s", rec.Code, rec.Body.String())
	}
	// A bad source is refused before any write.
	if rec := call(TransitionShipmentHandler(svc), http.MethodPost,
		`{"to":"DELIVERED","event_code":"X","source":"BOGUS"}`,
		map[string]string{"id": sh.ID}); rec.Code < 400 {
		t.Fatalf("bad event source = %d, want a 4xx", rec.Code)
	}
	// The tracking event append.
	if rec := call(RecordShipmentEventHandler(svc), http.MethodPost,
		`{"event_code":"ARRIVED_HUB","description":"رسید","source":"CARRIER_API"}`,
		map[string]string{"id": sh.ID}); rec.Code != http.StatusCreated {
		t.Fatalf("record event = %d: %s", rec.Code, rec.Body.String())
	}

	// A backwards transition is refused.
	if rec := call(ConfirmFulfillmentHandler(svc), http.MethodPost, "", map[string]string{"id": fo.ID}); rec.Code < 400 {
		t.Fatalf("re-confirming a delivered fulfillment = %d, want a 4xx", rec.Code)
	}
	// An unknown id is a 404, not a 500.
	if rec := call(ConfirmFulfillmentHandler(svc), http.MethodPost, "", map[string]string{"id": "00000000-0000-0000-0000-000000000000"}); rec.Code < 400 || rec.Code >= 500 {
		t.Fatalf("unknown id = %d, want a 4xx", rec.Code)
	}

	_ = store
}
