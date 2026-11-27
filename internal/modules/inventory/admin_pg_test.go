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

// TestPG_ListWarehousesHandler covers the panel's warehouse list: the status filter, the
// name/code search, the paging metadata and the 500 arm. The route did not exist before, so
// a warehouse could be created but never read back.
func TestPG_ListWarehousesHandler(t *testing.T) {
	pool, svc, _ := setupInventory(t)
	ctx := context.Background()

	for _, w := range []Warehouse{
		{WarehouseCode: "WH-L1", Name: "انبار یک", Status: WarehouseActive},
		{WarehouseCode: "WH-L2", Name: "انبار دو", Status: WarehouseInactive},
	} {
		if _, err := svc.CreateWarehouse(ctx, w); err != nil {
			t.Fatalf("create %s: %v", w.WarehouseCode, err)
		}
	}

	admin := NewAdminStore(pool)
	get := func(q string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/warehouses?"+q, nil)
		rec := httptest.NewRecorder()
		ListWarehousesHandler(admin)(rec, r)
		return rec
	}

	rec := get("")
	if rec.Code != http.StatusOK {
		t.Fatalf("list = %d: %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Items []WarehouseView `json:"items"`
		Page  struct {
			Total int `json:"total"`
		} `json:"page"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out.Items) < 2 || out.Page.Total < 2 {
		t.Fatalf("list body = %s", rec.Body.String())
	}
	// The code really comes back, which is what the panel needs to pick a warehouse.
	var sawCode bool
	for _, w := range out.Items {
		if w.WarehouseCode == "WH-L1" {
			sawCode = true
			if w.ID == "" {
				t.Fatal("a warehouse row has no id")
			}
		}
	}
	if !sawCode {
		t.Fatalf("WH-L1 not in the list: %s", rec.Body.String())
	}

	// The status filter narrows to the inactive one.
	_ = json.Unmarshal(get("status=INACTIVE").Body.Bytes(), &out)
	for _, w := range out.Items {
		if w.Status != WarehouseInactive {
			t.Fatalf("status filter leaked %s", w.Status)
		}
	}
	// The q filter matches the code.
	_ = json.Unmarshal(get("q=WH-L2").Body.Bytes(), &out)
	if len(out.Items) != 1 || out.Items[0].WarehouseCode != "WH-L2" {
		t.Fatalf("q filter = %s", get("q=WH-L2").Body.String())
	}
	// Nonsense paging falls back to the defaults.
	if rec := get("page=abc&page_size=xyz"); rec.Code != http.StatusOK {
		t.Fatalf("bad paging = %d", rec.Code)
	}

	// The 500 arm with a dead pool.
	dead := setupClosedPool(t)
	r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/warehouses", nil)
	rec = httptest.NewRecorder()
	ListWarehousesHandler(NewAdminStore(dead))(rec, r)
	if rec.Code != http.StatusInternalServerError || rec.Body.Len() == 0 {
		t.Fatalf("dead pool = %d, body %q", rec.Code, rec.Body.String())
	}
}
