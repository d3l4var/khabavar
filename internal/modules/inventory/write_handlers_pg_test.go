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

// TestPG_WriteHandlers covers the four write handlers end to end: warehouse creation with
// its validations, receive stock with the actor guard, adjust, and the reservation convert.
func TestPG_WriteHandlers(t *testing.T) {
	pool, svc, store := setupInventory(t)
	ctx := context.Background()
	orderItemID, variantID := seedOrderItem(t, ctx, pool, "KHA-ORD-2026-000960", "SKU-INV-WRITE")

	// stock_movements.created_by is a FK to admin_users, so the actor must be a real row.
	var adminID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO admin_users (email, full_name, password_hash, status)
		 VALUES ('inv-writer@khabavar.ir','نویسندهٔ انبار','x','ACTIVE') RETURNING id`).Scan(&adminID); err != nil {
		t.Fatalf("seed admin: %v", err)
	}

	call := func(h http.HandlerFunc, method, body string, pv map[string]string, actor string) *httptest.ResponseRecorder {
		var r *http.Request
		if body == "" {
			r = httptest.NewRequest(method, "/x", nil)
		} else {
			r = httptest.NewRequest(method, "/x", strings.NewReader(body))
		}
		for k, v := range pv {
			r.SetPathValue(k, v)
		}
		if actor != "" {
			r = r.WithContext(shared.WithActorID(r.Context(), actor))
		}
		rec := httptest.NewRecorder()
		h(rec, r)
		return rec
	}

	// --- create warehouse: validation, defaults, duplicate code ---
	if rec := call(CreateWarehouseHandler(svc), http.MethodPost, `{`, nil, ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("broken body = %d, want 400", rec.Code)
	}
	if rec := call(CreateWarehouseHandler(svc), http.MethodPost, `{"warehouse_code":"WH-X"}`, nil, ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("missing name = %d, want 400", rec.Code)
	}
	rec := call(CreateWarehouseHandler(svc), http.MethodPost,
		`{"name":"انبار نوشتن","warehouse_code":"WH-WRITE","address":"تهران"}`, nil, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("create warehouse = %d: %s", rec.Code, rec.Body.String())
	}
	var wh struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &wh)
	if wh.ID == "" {
		t.Fatalf("warehouse response = %s", rec.Body.String())
	}
	// The status defaulted to ACTIVE.
	w, err := store.GetWarehouse(ctx, wh.ID)
	if err != nil || w == nil || w.Status != WarehouseActive {
		t.Fatalf("warehouse = %+v, %v", w, err)
	}
	// The same code again is a conflict, not a 500.
	if rec := call(CreateWarehouseHandler(svc), http.MethodPost,
		`{"name":"تکراری","warehouse_code":"WH-WRITE"}`, nil, ""); rec.Code < 400 || rec.Code >= 500 {
		t.Fatalf("duplicate code = %d, want a 4xx", rec.Code)
	}

	// --- receive stock: the actor guard is the point (IAM §7) ---
	pv := map[string]string{"wid": wh.ID, "vid": variantID}
	if rec := call(ReceiveStockHandler(svc), http.MethodPost, `{`, pv, adminID); rec.Code != http.StatusBadRequest {
		t.Fatalf("broken receive = %d, want 400", rec.Code)
	}
	// No actor in the context: 401, and nothing is written.
	if rec := call(ReceiveStockHandler(svc), http.MethodPost, `{"quantity":5}`, pv, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("receive without an actor = %d, want 401", rec.Code)
	}
	if bal, _ := store.GetStockBalance(ctx, wh.ID, variantID); bal != nil && bal.OnHandQuantity != 0 {
		t.Fatalf("a refused receive still moved stock: %+v", bal)
	}
	// With an actor it lands, and the movement records that actor.
	if rec := call(ReceiveStockHandler(svc), http.MethodPost,
		`{"quantity":12,"reference_type":"PURCHASE","reference_id":"`+orderItemID+`"}`, pv, adminID); rec.Code != http.StatusOK {
		t.Fatalf("receive = %d: %s", rec.Code, rec.Body.String())
	}
	bal, err := store.GetStockBalance(ctx, wh.ID, variantID)
	if err != nil || bal == nil || bal.OnHandQuantity != 12 {
		t.Fatalf("balance after receive = %+v, %v", bal, err)
	}
	movs, err := svc.ListMovements(ctx, wh.ID, variantID, 10)
	if err != nil || len(movs) == 0 {
		t.Fatalf("movements = %+v, %v", movs, err)
	}
	if movs[0].CreatedBy == "" {
		t.Fatal("the receipt movement has no actor")
	}
	// A zero quantity is a 400.
	if rec := call(ReceiveStockHandler(svc), http.MethodPost, `{"quantity":0}`, pv, adminID); rec.Code != http.StatusBadRequest {
		t.Fatalf("zero receive = %d, want 400", rec.Code)
	}

	// --- adjust: the same actor rule, and a negative delta is allowed ---
	if rec := call(AdjustStockHandler(svc), http.MethodPost, `{"delta":-2,"notes":"شکستگی"}`, pv, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("adjust without an actor = %d, want 401", rec.Code)
	}
	if rec := call(AdjustStockHandler(svc), http.MethodPost, `{"delta":-2,"notes":"شکستگی"}`, pv, adminID); rec.Code != http.StatusOK {
		t.Fatalf("adjust = %d: %s", rec.Code, rec.Body.String())
	}
	bal, _ = store.GetStockBalance(ctx, wh.ID, variantID)
	if bal.OnHandQuantity != 10 {
		t.Fatalf("on_hand after adjust = %d, want 10", bal.OnHandQuantity)
	}
	// A delta of zero is refused.
	if rec := call(AdjustStockHandler(svc), http.MethodPost, `{"delta":0}`, pv, adminID); rec.Code != http.StatusBadRequest {
		t.Fatalf("zero delta = %d, want 400", rec.Code)
	}

	// --- convert reservation through the handler ---
	resID, err := svc.ReserveForOrder(ctx, Reservation{
		WarehouseID: wh.ID, VariantID: variantID, Quantity: 3,
		OrderItemID: orderItemID, ExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if rec := call(ConvertReservationHandler(svc), http.MethodPost, "", map[string]string{"id": resID}, ""); rec.Code != http.StatusOK {
		t.Fatalf("convert = %d: %s", rec.Code, rec.Body.String())
	}
	bal, _ = store.GetStockBalance(ctx, wh.ID, variantID)
	if bal.OnHandQuantity != 7 {
		t.Fatalf("on_hand after convert = %d, want 7", bal.OnHandQuantity)
	}
	// Converting again is a 409.
	if rec := call(ConvertReservationHandler(svc), http.MethodPost, "", map[string]string{"id": resID}, ""); rec.Code != http.StatusConflict {
		t.Fatalf("double convert = %d, want 409", rec.Code)
	}
	// An unknown reservation is a 404.
	if rec := call(ConvertReservationHandler(svc), http.MethodPost, "",
		map[string]string{"id": "00000000-0000-0000-0000-000000000000"}, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("convert unknown = %d, want 404", rec.Code)
	}

	// --- parsePositiveInt through the movement list's limit ---
	for _, q := range []string{"", "limit=abc", "limit=0", "limit=-5", "limit=7"} {
		r := httptest.NewRequest(http.MethodGet, "/x?"+q, nil)
		r.SetPathValue("wid", wh.ID)
		r.SetPathValue("vid", variantID)
		rec := httptest.NewRecorder()
		ListMovementsHandler(svc)(rec, r)
		if rec.Code != http.StatusOK {
			t.Fatalf("movements with %q = %d: %s", q, rec.Code, rec.Body.String())
		}
	}
}

// TestPG_ReserveHandlerAndAdminReads covers the reserve handler's validation, the admin
// reservation list (including its RFC3339 guard) and the movement error writer.
func TestPG_ReserveHandlerAndAdminReads(t *testing.T) {
	pool, svc, store := setupInventory(t)
	ctx := context.Background()
	orderItemID, variantID := seedOrderItem(t, ctx, pool, "KHA-ORD-2026-000970", "SKU-INV-RES")

	whID, err := svc.CreateWarehouse(ctx, Warehouse{
		WarehouseCode: "WH-RES", Name: "انبار رزرو", Status: WarehouseActive,
	})
	if err != nil {
		t.Fatalf("warehouse: %v", err)
	}
	if err := store.UpsertStockBalance(ctx, StockBalance{
		WarehouseID: whID, VariantID: variantID, OnHandQuantity: 5,
	}); err != nil {
		t.Fatalf("balance: %v", err)
	}

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

	// A broken body and a missing order_item_id are 400s (the second used to be a 500).
	if rec := call(ReserveForOrderHandler(svc), http.MethodPost, `{`, nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("broken reserve = %d, want 400", rec.Code)
	}
	if rec := call(ReserveForOrderHandler(svc), http.MethodPost,
		`{"warehouse_id":"`+whID+`","variant_id":"`+variantID+`","quantity":1}`, nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("reserve without order_item_id = %d, want 400: %s", rec.Code, rec.Body.String())
	}
	// Over-reserving is a 409 (the sellable invariant).
	if rec := call(ReserveForOrderHandler(svc), http.MethodPost,
		`{"warehouse_id":"`+whID+`","variant_id":"`+variantID+`","order_item_id":"`+orderItemID+`",`+
			`"quantity":99,"expires_at":"2030-01-01T00:00:00Z"}`,
		nil); rec.Code != http.StatusConflict {
		t.Fatalf("over-reserve = %d, want 409: %s", rec.Code, rec.Body.String())
	}
	// A real reservation, with an RFC3339 expiry.
	rec := call(ReserveForOrderHandler(svc), http.MethodPost,
		`{"warehouse_id":"`+whID+`","variant_id":"`+variantID+`","order_item_id":"`+orderItemID+`",`+
			`"quantity":2,"expires_at":"2030-01-01T00:00:00Z"}`, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("reserve = %d: %s", rec.Code, rec.Body.String())
	}
	// A garbage expiry is ignored rather than erroring, and the service refuses it for lack
	// of a usable expiry: either way it is not a 500.
	if rec := call(ReserveForOrderHandler(svc), http.MethodPost,
		`{"warehouse_id":"`+whID+`","variant_id":"`+variantID+`","order_item_id":"`+orderItemID+`",`+
			`"quantity":1,"expires_at":"not-a-date"}`, nil); rec.Code >= 500 {
		t.Fatalf("garbage expiry = %d, want a 4xx", rec.Code)
	}

	// The admin reservation list: a bad expires_before is a 400 with a code.
	admin := NewAdminStore(pool)
	if rec := call(ListReservationsHandler(admin), http.MethodGet, "", nil); rec.Code != http.StatusOK {
		t.Fatalf("list reservations = %d: %s", rec.Code, rec.Body.String())
	}
	r := httptest.NewRequest(http.MethodGet, "/x?expires_before=not-a-date", nil)
	rec2 := httptest.NewRecorder()
	ListReservationsHandler(admin)(rec2, r)
	if rec2.Code != http.StatusBadRequest || !strings.Contains(rec2.Body.String(), "invalid_request") {
		t.Fatalf("bad expires_before = %d: %s", rec2.Code, rec2.Body.String())
	}
	// A valid filter and paging work.
	r = httptest.NewRequest(http.MethodGet, "/x?expires_before=2030-01-01T00:00:00Z&page=1&page_size=5", nil)
	rec3 := httptest.NewRecorder()
	ListReservationsHandler(admin)(rec3, r)
	if rec3.Code != http.StatusOK {
		t.Fatalf("filtered reservations = %d: %s", rec3.Code, rec3.Body.String())
	}

	// The stock balance read for a real balance returns on_hand and reserved.
	r = httptest.NewRequest(http.MethodGet, "/x", nil)
	r.SetPathValue("wid", whID)
	r.SetPathValue("vid", variantID)
	rec4 := httptest.NewRecorder()
	GetStockBalanceHandler(svc)(rec4, r)
	if rec4.Code != http.StatusOK {
		t.Fatalf("balance = %d: %s", rec4.Code, rec4.Body.String())
	}
	// The handler wraps the balance in {"data": ...}.
	var bal struct {
		Data struct {
			OnHand   int `json:"on_hand_quantity"`
			Reserved int `json:"reserved_quantity"`
		} `json:"data"`
	}
	_ = json.Unmarshal(rec4.Body.Bytes(), &bal)
	if bal.Data.OnHand != 5 || bal.Data.Reserved != 2 {
		t.Fatalf("balance body = %s", rec4.Body.String())
	}
}
