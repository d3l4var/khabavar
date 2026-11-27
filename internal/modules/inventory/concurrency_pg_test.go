package inventory

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/khabavar/khabavar/internal/modules/shared"
)

// TestPG_ReserveStockConcurrency is the one place in the module where real contention
// happens: two reservations racing for the last units. SELECT ... FOR UPDATE must serialise
// them so the sellable invariant (on_hand - reserved >= 0) survives.
func TestPG_ReserveStockConcurrency(t *testing.T) {
	pool, svc, store := setupInventory(t)
	ctx := context.Background()
	orderItemID, variantID := seedOrderItem(t, ctx, pool, "KHA-ORD-2026-000980", "SKU-INV-RACE")

	whID, err := svc.CreateWarehouse(ctx, Warehouse{
		WarehouseCode: "WH-RACE", Name: "انبار رقابت", Status: WarehouseActive,
	})
	if err != nil {
		t.Fatalf("warehouse: %v", err)
	}
	if err := store.UpsertStockBalance(ctx, StockBalance{
		WarehouseID: whID, VariantID: variantID, OnHandQuantity: 3,
	}); err != nil {
		t.Fatalf("balance: %v", err)
	}

	// Five goroutines each want 1 unit; only three can win.
	const want = 5
	var wg sync.WaitGroup
	results := make([]error, want)
	for i := 0; i < want; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = store.ReserveStock(ctx, whID, variantID, 1, time.Now())
		}(i)
	}
	wg.Wait()

	var ok, insufficient int
	for _, err := range results {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, ErrInsufficientStock):
			insufficient++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if ok != 3 || insufficient != 2 {
		t.Fatalf("reservations: %d ok, %d insufficient — want 3 and 2", ok, insufficient)
	}
	// The invariant holds: reserved never exceeded on_hand.
	bal, err := store.GetStockBalance(ctx, whID, variantID)
	if err != nil || bal == nil {
		t.Fatalf("balance = %+v, %v", bal, err)
	}
	if bal.ReservedQuantity != 3 || bal.OnHandQuantity != 3 {
		t.Fatalf("after the race: on_hand=%d reserved=%d", bal.OnHandQuantity, bal.ReservedQuantity)
	}
	if bal.OnHandQuantity-bal.ReservedQuantity < 0 {
		t.Fatal("the sellable invariant was violated")
	}
	_ = orderItemID
}

// TestPG_ReserveStockCreatesAZeroRow covers the no-row branch: reserving against a variant
// with no balance row creates one and then fails the invariant rather than erroring.
func TestPG_ReserveStockCreatesAZeroRow(t *testing.T) {
	pool, svc, store := setupInventory(t)
	ctx := context.Background()
	_, variantID := seedOrderItem(t, ctx, pool, "KHA-ORD-2026-000981", "SKU-INV-ZERO")

	whID, err := svc.CreateWarehouse(ctx, Warehouse{
		WarehouseCode: "WH-ZERO", Name: "انبار صفر", Status: WarehouseActive,
	})
	if err != nil {
		t.Fatalf("warehouse: %v", err)
	}
	// No balance row exists yet.
	if err := store.ReserveStock(ctx, whID, variantID, 1, time.Now()); !errors.Is(err, ErrInsufficientStock) {
		t.Fatalf("reserve with no row = %v, want ErrInsufficientStock", err)
	}
	// The zero row is inserted inside the transaction and then ROLLED BACK when the
	// invariant fails: a refused reservation must not leave a row behind. The code comment
	// claimed the row was created; the behaviour is the safer one, so the comment was fixed
	// and this pins the real outcome.
	bal, err := store.GetStockBalance(ctx, whID, variantID)
	if err != nil {
		t.Fatalf("balance: %v", err)
	}
	if bal != nil {
		t.Fatalf("a refused reservation left a row behind: %+v", bal)
	}

	// ReleaseStock on a missing row is a no-op, not an error.
	if err := store.ReleaseStock(ctx, whID, "00000000-0000-0000-0000-0000000000ff", 1, time.Now()); err != nil {
		t.Fatalf("release on a missing row = %v", err)
	}
}

// TestWriteMovementErrPinsStatusCodes pins the movement error mapping, which was at 50%.
func TestWriteMovementErrPinsStatusCodes(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{ErrZeroQuantity, http.StatusBadRequest},
		{ErrMovementTypeNotAllowed, http.StatusBadRequest},
		{ErrNegativeOnHand, http.StatusConflict},
		{ErrInsufficientStock, http.StatusConflict},
		{errors.New("boom"), http.StatusInternalServerError},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		writeMovementErr(rec, c.err)
		if rec.Code != c.want {
			t.Fatalf("%v = %d, want %d", c.err, rec.Code, c.want)
		}
		if rec.Body.Len() == 0 {
			t.Fatalf("%v answered an empty body", c.err)
		}
	}
}

// TestPG_ExpireReservationsAndZeroBalance covers the expiry sweep and the zero-balance
// fallback: an expired reservation is released and its movement posted, and a variant with
// no row reads as a zero balance rather than a 404.
func TestPG_ExpireReservationsAndZeroBalance(t *testing.T) {
	pool, svc, store := setupInventory(t)
	ctx := context.Background()
	orderItemID, variantID := seedOrderItem(t, ctx, pool, "KHA-ORD-2026-000990", "SKU-INV-EXP")

	whID, err := svc.CreateWarehouse(ctx, Warehouse{
		WarehouseCode: "WH-EXP", Name: "انبار انقضا", Status: WarehouseActive,
	})
	if err != nil {
		t.Fatalf("warehouse: %v", err)
	}
	if err := store.UpsertStockBalance(ctx, StockBalance{
		WarehouseID: whID, VariantID: variantID, OnHandQuantity: 6,
	}); err != nil {
		t.Fatalf("balance: %v", err)
	}

	// A reservation that expired an hour ago.
	resID, err := svc.ReserveForOrder(ctx, Reservation{
		WarehouseID: whID, VariantID: variantID, Quantity: 2,
		OrderItemID: orderItemID, ExpiresAt: time.Now().Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	bal, _ := store.GetStockBalance(ctx, whID, variantID)
	if bal.ReservedQuantity != 2 {
		t.Fatalf("reserved before the sweep = %d, want 2", bal.ReservedQuantity)
	}

	n, err := svc.ExpireReservations(ctx, 0)
	if err != nil {
		t.Fatalf("expire: %v", err)
	}
	if n < 1 {
		t.Fatalf("the sweep expired %d reservations, want at least 1", n)
	}
	// The stock came back and a RELEASE movement was posted.
	bal, _ = store.GetStockBalance(ctx, whID, variantID)
	if bal.ReservedQuantity != 0 {
		t.Fatalf("reserved after the sweep = %d, want 0", bal.ReservedQuantity)
	}
	movs, err := svc.ListMovements(ctx, whID, variantID, 20)
	if err != nil {
		t.Fatalf("movements: %v", err)
	}
	var sawRelease bool
	for _, m := range movs {
		if m.MovementType == MovementRelease && m.Quantity == -2 {
			sawRelease = true
		}
	}
	if !sawRelease {
		t.Fatalf("no RELEASE movement for the expired reservation: %+v", movs)
	}
	// A second sweep finds nothing more for it.
	if n2, err := svc.ExpireReservations(ctx, 100); err != nil || n2 != 0 {
		t.Fatalf("second sweep = %d, %v", n2, err)
	}
	_ = resID

	// A variant with no balance row reads as zero, not as a 404.
	zero, err := svc.GetStockBalance(ctx, whID, "00000000-0000-0000-0000-0000000000ee")
	if err != nil {
		t.Fatalf("zero balance: %v", err)
	}
	if zero == nil || zero.OnHandQuantity != 0 || zero.ReservedQuantity != 0 {
		t.Fatalf("zero balance = %+v", zero)
	}
	// Empty ids are a sentinel, not a query.
	if _, err := svc.GetStockBalance(ctx, "", variantID); !errors.Is(err, ErrStockBalanceNotFound) {
		t.Fatalf("empty warehouse = %v", err)
	}
}

// TestPG_ReceiveAndReleaseEdgeBranches covers the remaining service branches: a receipt
// against an unknown warehouse, and a reservation release through its handler.
func TestPG_ReceiveAndReleaseEdgeBranches(t *testing.T) {
	pool, svc, store := setupInventory(t)
	ctx := context.Background()
	orderItemID, variantID := seedOrderItem(t, ctx, pool, "KHA-ORD-2026-000995", "SKU-INV-EDGE2")

	var adminID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO admin_users (email, full_name, password_hash, status)
		 VALUES ('inv-edge@khabavar.ir','لبه','x','ACTIVE') RETURNING id`).Scan(&adminID); err != nil {
		t.Fatalf("seed admin: %v", err)
	}

	// An unknown warehouse is a sentinel, not a foreign-key 500.
	if err := svc.ReceiveStock(ctx, "00000000-0000-0000-0000-000000000000", variantID, 1,
		"PURCHASE", orderItemID, adminID); !errors.Is(err, ErrWarehouseNotFound) {
		t.Fatalf("receive into an unknown warehouse = %v, want ErrWarehouseNotFound", err)
	}

	whID, err := svc.CreateWarehouse(ctx, Warehouse{
		WarehouseCode: "WH-EDGE2", Name: "انبار لبه۲", Status: WarehouseActive,
	})
	if err != nil {
		t.Fatalf("warehouse: %v", err)
	}
	if err := store.UpsertStockBalance(ctx, StockBalance{
		WarehouseID: whID, VariantID: variantID, OnHandQuantity: 4,
	}); err != nil {
		t.Fatalf("balance: %v", err)
	}
	resID, err := svc.ReserveForOrder(ctx, Reservation{
		WarehouseID: whID, VariantID: variantID, Quantity: 2,
		OrderItemID: orderItemID, ExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}

	// Release through the handler: 200, and the reserved count drops.
	r := httptest.NewRequest(http.MethodDelete, "/x", nil)
	r.SetPathValue("id", resID)
	rec := httptest.NewRecorder()
	ReleaseReservationHandler(svc)(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("release = %d: %s", rec.Code, rec.Body.String())
	}
	bal, _ := store.GetStockBalance(ctx, whID, variantID)
	if bal.ReservedQuantity != 0 {
		t.Fatalf("reserved after release = %d", bal.ReservedQuantity)
	}
	// Releasing it again is a 409, and an unknown id is a 404.
	r = httptest.NewRequest(http.MethodDelete, "/x", nil)
	r.SetPathValue("id", resID)
	rec = httptest.NewRecorder()
	ReleaseReservationHandler(svc)(rec, r)
	if rec.Code != http.StatusConflict {
		t.Fatalf("double release = %d, want 409", rec.Code)
	}
	r = httptest.NewRequest(http.MethodDelete, "/x", nil)
	r.SetPathValue("id", "00000000-0000-0000-0000-000000000000")
	rec = httptest.NewRecorder()
	ReleaseReservationHandler(svc)(rec, r)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("release unknown = %d, want 404", rec.Code)
	}
}

// TestPG_RecordMovementHandler covers the manual movement route: the required fields, the
// MANUAL default for reference_type, a real write, and the conflict when the ledger would go
// negative.
func TestPG_RecordMovementHandler(t *testing.T) {
	pool, svc, store := setupInventory(t)
	ctx := context.Background()
	_, variantID := seedOrderItem(t, ctx, pool, "KHA-ORD-2026-000997", "SKU-INV-MOV")

	var adminID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO admin_users (email, full_name, password_hash, status)
		 VALUES ('inv-mov@khabavar.ir','حرکت','x','ACTIVE') RETURNING id`).Scan(&adminID); err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	whID, err := svc.CreateWarehouse(ctx, Warehouse{
		WarehouseCode: "WH-MOV", Name: "انبار حرکت", Status: WarehouseActive,
	})
	if err != nil {
		t.Fatalf("warehouse: %v", err)
	}
	if err := store.UpsertStockBalance(ctx, StockBalance{
		WarehouseID: whID, VariantID: variantID, OnHandQuantity: 5,
	}); err != nil {
		t.Fatalf("balance: %v", err)
	}

	call := func(body string, actor string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(body))
		if actor != "" {
			r = r.WithContext(shared.WithActorID(r.Context(), actor))
		}
		rec := httptest.NewRecorder()
		RecordMovementHandler(svc)(rec, r)
		return rec
	}

	if rec := call(`{`, adminID); rec.Code != http.StatusBadRequest {
		t.Fatalf("broken body = %d, want 400", rec.Code)
	}
	// Each required field is enforced.
	if rec := call(`{"variant_id":"`+variantID+`","movement_type":"ADJUSTMENT"}`, adminID); rec.Code != http.StatusBadRequest {
		t.Fatalf("missing warehouse = %d, want 400", rec.Code)
	}
	if rec := call(`{"warehouse_id":"`+whID+`","variant_id":"`+variantID+`"}`, adminID); rec.Code != http.StatusBadRequest {
		t.Fatalf("missing movement_type = %d, want 400", rec.Code)
	}
	// A zero quantity is a 400 through writeMovementErr.
	if rec := call(`{"warehouse_id":"`+whID+`","variant_id":"`+variantID+`","movement_type":"ADJUSTMENT","quantity":0}`, adminID); rec.Code != http.StatusBadRequest {
		t.Fatalf("zero quantity = %d, want 400", rec.Code)
	}
	// A real movement: reference_type defaults to MANUAL.
	rec := call(`{"warehouse_id":"`+whID+`","variant_id":"`+variantID+`","movement_type":"ADJUSTMENT","quantity":-2,"note":"شکستگی"}`, adminID)
	if rec.Code != http.StatusCreated {
		t.Fatalf("record movement = %d: %s", rec.Code, rec.Body.String())
	}
	movs, err := svc.ListMovements(ctx, whID, variantID, 10)
	if err != nil || len(movs) == 0 {
		t.Fatalf("movements = %+v, %v", movs, err)
	}
	if movs[0].ReferenceType != "MANUAL" {
		t.Fatalf("reference_type = %q, want MANUAL", movs[0].ReferenceType)
	}
	// Taking more than on_hand is a conflict, not a 500.
	if rec := call(`{"warehouse_id":"`+whID+`","variant_id":"`+variantID+`","movement_type":"ADJUSTMENT","quantity":-99}`, adminID); rec.Code != http.StatusConflict {
		t.Fatalf("over-draw = %d, want 409: %s", rec.Code, rec.Body.String())
	}
}

// TestPG_ServiceSurfacesDriverErrors walks every service entry point against a closed pool.
// Each one must return the driver error rather than panicking or swallowing it — these are
// the `return err` arms after each store call, which no happy-path test reaches.
func TestPG_ServiceSurfacesDriverErrors(t *testing.T) {
	pool := setupClosedPool(t)
	svc := NewService(NewDBStore(pool))
	ctx := context.Background()
	now := time.Now()

	calls := map[string]func() error{
		"create warehouse": func() error {
			_, err := svc.CreateWarehouse(ctx, Warehouse{WarehouseCode: "W", Name: "n", Status: WarehouseActive})
			return err
		},
		"get balance": func() error {
			_, err := svc.GetStockBalance(ctx, "w", "v")
			return err
		},
		"adjust stock": func() error {
			return svc.AdjustStock(ctx, "w", "v", 1, "ADJUSTMENT", "", "", "actor")
		},
		"receive stock": func() error {
			return svc.ReceiveStock(ctx, "w", "v", 1, "PURCHASE", "", "actor")
		},
		"reserve": func() error {
			_, err := svc.ReserveForOrder(ctx, Reservation{
				WarehouseID: "w", VariantID: "v", OrderItemID: "o", Quantity: 1,
				ExpiresAt: now.Add(time.Hour),
			})
			return err
		},
		"release": func() error { return svc.ReleaseReservation(ctx, "r") },
		"convert": func() error { return svc.ConvertReservation(ctx, "r") },
		"list movements": func() error {
			_, err := svc.ListMovements(ctx, "w", "v", 10)
			return err
		},
		"expire": func() error {
			_, err := svc.ExpireReservations(ctx, 10)
			return err
		},
	}
	for name, fn := range calls {
		if err := fn(); err == nil {
			t.Fatalf("%s against a closed pool returned no error", name)
		}
	}
}
