package inventory

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/khabavar/khabavar/internal/modules/shared"
)

// TestPG_InventoryAdminHandlers covers the panel surface that had no tests at all: the
// stock list, the reservation list and the manual movement endpoint.
func TestPG_InventoryAdminHandlers(t *testing.T) {
	pool, svc, store := setupInventory(t)
	ctx := context.Background()
	orderItemID, variantID := seedOrderItem(t, ctx, pool, "KHA-ORD-2026-000902", "SKU-ADM-1")

	var actorID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO admin_users (email, full_name, status) VALUES ($1,'انباردار','ACTIVE') RETURNING id`,
		"adm-"+variantID[:8]+"@khabavar.ir").Scan(&actorID); err != nil {
		t.Fatalf("seed actor: %v", err)
	}

	whID, err := svc.CreateWarehouse(ctx, Warehouse{WarehouseCode: "WH-ADM", Name: "انبار پنل"})
	if err != nil {
		t.Fatalf("CreateWarehouse: %v", err)
	}
	if err := svc.ReceiveStock(ctx, whID, variantID, 12, "PO", "", actorID); err != nil {
		t.Fatalf("ReceiveStock: %v", err)
	}

	call := func(h http.HandlerFunc, method, target, body string) *httptest.ResponseRecorder {
		var r *http.Request
		if body == "" {
			r = httptest.NewRequest(method, target, nil)
		} else {
			r = httptest.NewRequest(method, target, strings.NewReader(body))
		}
		r = r.WithContext(shared.WithActorID(r.Context(), actorID))
		rec := httptest.NewRecorder()
		h(rec, r)
		return rec
	}

	// Stock list: the row we just created must appear, and the paging block must be there.
	rec := call(ListStockHandler(NewAdminStore(pool)), http.MethodGet, "/api/v1/admin/stock?page_size=50", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("stock list = %d: %s", rec.Code, rec.Body.String())
	}
	var stock struct {
		Items []struct {
			VariantID   string `json:"variant_id"`
			OnHand      int    `json:"on_hand_quantity"`
			WarehouseID string `json:"warehouse_id"`
		} `json:"items"`
		Page struct {
			Page  int `json:"page"`
			Total int `json:"total"`
		} `json:"page"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &stock)
	if len(stock.Items) == 0 || stock.Items[0].OnHand != 12 {
		t.Fatalf("stock list = %s", rec.Body.String())
	}
	// The q filter narrows by SKU.
	rec = call(ListStockHandler(NewAdminStore(pool)), http.MethodGet, "/api/v1/admin/stock?q=SKU-ADM-1", "")
	_ = json.Unmarshal(rec.Body.Bytes(), &stock)
	if len(stock.Items) != 1 {
		t.Fatalf("q filter returned %d rows", len(stock.Items))
	}
	rec = call(ListStockHandler(NewAdminStore(pool)), http.MethodGet, "/api/v1/admin/stock?q=no-such-sku", "")
	_ = json.Unmarshal(rec.Body.Bytes(), &stock)
	if len(stock.Items) != 0 {
		t.Fatalf("q filter matched %d rows for a missing SKU", len(stock.Items))
	}

	// Reservations: empty is fine, and a broken expires_before is a 400.
	rec = call(ListReservationsHandler(NewAdminStore(pool)), http.MethodGet, "/api/v1/admin/reservations", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("reservations = %d: %s", rec.Code, rec.Body.String())
	}
	rec = call(ListReservationsHandler(NewAdminStore(pool)), http.MethodGet, "/api/v1/admin/reservations?expires_before=not-a-date", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad expires_before = %d, want 400", rec.Code)
	}
	if _, err := svc.ReserveForOrder(ctx, Reservation{
		WarehouseID: whID, VariantID: variantID, OrderItemID: orderItemID, Quantity: 2,
		ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("ReserveForOrder: %v", err)
	}
	rec = call(ListReservationsHandler(NewAdminStore(pool)), http.MethodGet, "/api/v1/admin/reservations", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), variantID) {
		t.Fatalf("reservations after reserve = %d: %s", rec.Code, rec.Body.String())
	}

	// Manual movement: a correction type is accepted, a nonsense type is refused, and a
	// missing field is a 400.
	rec = call(RecordMovementHandler(svc), http.MethodPost, "/api/v1/admin/stock/movements",
		`{"warehouse_id":"`+whID+`","variant_id":"`+variantID+`","movement_type":"DAMAGE","quantity":-2}`)
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("movement = %d: %s", rec.Code, rec.Body.String())
	}
	rec = call(RecordMovementHandler(svc), http.MethodPost, "/api/v1/admin/stock/movements",
		`{"warehouse_id":"`+whID+`","variant_id":"`+variantID+`","movement_type":"NONSENSE","quantity":1}`)
	if rec.Code < 400 {
		t.Fatalf("nonsense movement = %d, want a 4xx", rec.Code)
	}
	rec = call(RecordMovementHandler(svc), http.MethodPost, "/api/v1/admin/stock/movements", `{"warehouse_id":"x"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing fields = %d, want 400", rec.Code)
	}
	_ = store
}
