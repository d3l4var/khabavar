package inventory

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/khabavar/khabavar/internal/modules/shared"
)

// TestPG_InventoryHTTPHandlers drives the whole inventory surface through its handlers
// against the real schema: warehouse creation, receipt, balance, adjustment, reservation
// lifecycle and the movement ledger. The service tests cannot see a wrong status code or a
// handler that drops a path value.
func TestPG_InventoryHTTPHandlers(t *testing.T) {
	pool, svc, _ := setupInventory(t)
	ctx := context.Background()
	orderItemID, variantID := seedOrderItem(t, ctx, pool, "KHA-ORD-2026-000901", "SKU-INVH-1")

	// A real staff account: stock_movements.created_by references admin_users.
	var actorID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO admin_users (email, full_name, status) VALUES ($1,'انباردار','ACTIVE') RETURNING id`,
		"keeper-"+variantID[:8]+"@khabavar.ir").Scan(&actorID); err != nil {
		t.Fatalf("seed actor: %v", err)
	}

	call := func(h http.HandlerFunc, method, body string, pv map[string]string) *httptest.ResponseRecorder {
		var r *http.Request
		if body == "" {
			r = httptest.NewRequest(method, "/x", nil)
		} else {
			r = httptest.NewRequest(method, "/x", strings.NewReader(body))
		}
		r = r.WithContext(shared.WithActorID(r.Context(), actorID))
		for k, v := range pv {
			r.SetPathValue(k, v)
		}
		rec := httptest.NewRecorder()
		h(rec, r)
		return rec
	}

	// Warehouse.
	rec := call(CreateWarehouseHandler(svc), http.MethodPost, `{"name":"انبار تست","warehouse_code":"WHH1"}`, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create warehouse = %d: %s", rec.Code, rec.Body.String())
	}
	var wh struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &wh)

	// Receipt.
	rec = call(ReceiveStockHandler(svc), http.MethodPost, `{"quantity":10,"reference_type":"PO","reference_id":"`+uuid.NewString()+`"}`,
		map[string]string{"wid": wh.ID, "vid": variantID})
	if rec.Code != http.StatusOK && rec.Code != http.StatusCreated {
		t.Fatalf("receive = %d: %s", rec.Code, rec.Body.String())
	}

	// Balance must show on_hand 10 and available 10.
	rec = call(GetStockBalanceHandler(svc), http.MethodGet, "", map[string]string{"wid": wh.ID, "vid": variantID})
	if rec.Code != http.StatusOK {
		t.Fatalf("balance = %d: %s", rec.Code, rec.Body.String())
	}
	var bal struct {
		Data struct {
			OnHand   int `json:"on_hand_quantity"`
			Reserved int `json:"reserved_quantity"`
		} `json:"data"`
	}
	available := func() int { return bal.Data.OnHand - bal.Data.Reserved }
	_ = json.Unmarshal(rec.Body.Bytes(), &bal)
	if bal.Data.OnHand != 10 || available() != 10 {
		t.Fatalf("balance = %+v", bal.Data)
	}

	// Adjustment down.
	rec = call(AdjustStockHandler(svc), http.MethodPost, `{"delta":-4,"reference_type":"MANUAL","created_by":"admin-1"}`,
		map[string]string{"wid": wh.ID, "vid": variantID})
	if rec.Code != http.StatusOK {
		t.Fatalf("adjust = %d: %s", rec.Code, rec.Body.String())
	}
	// A zero delta is refused.
	rec = call(AdjustStockHandler(svc), http.MethodPost, `{"delta":0}`, map[string]string{"wid": wh.ID, "vid": variantID})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("zero adjust = %d, want 400", rec.Code)
	}

	// Reserve 3, then release it.
	rec = call(ReserveForOrderHandler(svc), http.MethodPost,
		`{"warehouse_id":"`+wh.ID+`","variant_id":"`+variantID+`","order_item_id":"`+orderItemID+`","quantity":3,"expires_at":"2030-01-01T00:00:00Z"}`, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("reserve = %d: %s", rec.Code, rec.Body.String())
	}
	var rsv struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &rsv)

	rec = call(GetStockBalanceHandler(svc), http.MethodGet, "", map[string]string{"wid": wh.ID, "vid": variantID})
	_ = json.Unmarshal(rec.Body.Bytes(), &bal)
	if available() != 3 {
		t.Fatalf("available after reserve = %d, want 3", available())
	}
	if rec = call(ReleaseReservationHandler(svc), http.MethodDelete, "", map[string]string{"id": rsv.ID}); rec.Code != http.StatusOK {
		t.Fatalf("release = %d: %s", rec.Code, rec.Body.String())
	}
	if rec = call(ReleaseReservationHandler(svc), http.MethodDelete, "", map[string]string{"id": rsv.ID}); rec.Code < 400 {
		t.Fatalf("releasing twice = %d, want a 4xx", rec.Code)
	}

	// Reserve again and convert it: the stock must actually drop.
	rec = call(ReserveForOrderHandler(svc), http.MethodPost,
		`{"warehouse_id":"`+wh.ID+`","variant_id":"`+variantID+`","order_item_id":"`+orderItemID+`","quantity":2,"expires_at":"2030-01-01T00:00:00Z"}`, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("reserve 2 = %d: %s", rec.Code, rec.Body.String())
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &rsv)
	if rec = call(ConvertReservationHandler(svc), http.MethodPost, "", map[string]string{"id": rsv.ID}); rec.Code != http.StatusOK {
		t.Fatalf("convert = %d: %s", rec.Code, rec.Body.String())
	}
	rec = call(GetStockBalanceHandler(svc), http.MethodGet, "", map[string]string{"wid": wh.ID, "vid": variantID})
	_ = json.Unmarshal(rec.Body.Bytes(), &bal)
	if bal.Data.OnHand != 4 {
		t.Fatalf("on_hand after convert = %d, want 4", bal.Data.OnHand)
	}

	// The movement ledger must have every step.
	rec = call(ListMovementsHandler(svc), http.MethodGet, "", map[string]string{"wid": wh.ID, "vid": variantID})
	if rec.Code != http.StatusOK {
		t.Fatalf("movements = %d: %s", rec.Code, rec.Body.String())
	}
	var mv struct {
		Data []Movement `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &mv)
	if len(mv.Data) < 3 {
		t.Fatalf("movements = %d, want at least 3 (receipt, adjustment, convert)", len(mv.Data))
	}
}
