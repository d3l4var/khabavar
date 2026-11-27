package inventory

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khabavar/khabavar/internal/platform/testpg"
)

// TestErrorMappingPinsStatusCodes pins the HTTP status for each sentinel. A wrong status
// here is a client contract bug that no happy-path test would catch.
func TestErrorMappingPinsStatusCodes(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{ErrWarehouseNotFound, http.StatusNotFound},
		{ErrStockBalanceNotFound, http.StatusNotFound},
		{ErrReservationNotFound, http.StatusNotFound},
		{ErrInsufficientStock, http.StatusConflict},
		{ErrReservationNotActive, http.StatusConflict},
		{ErrReservationExpiryReq, http.StatusBadRequest},
		{ErrZeroQuantity, http.StatusBadRequest},
		{ErrWarehouseCodeReq, http.StatusBadRequest},
		{ErrWarehouseNameReq, http.StatusBadRequest},
		{ErrDuplicateWarehouseCode, http.StatusConflict},
		{errors.New("something unexpected"), http.StatusInternalServerError},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		writeInvSvcErr(rec, c.err)
		if rec.Code != c.want {
			t.Fatalf("%v = %d, want %d", c.err, rec.Code, c.want)
		}
		// Every branch answers JSON, never an empty body.
		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body["error"] == nil {
			t.Fatalf("%v body = %s", c.err, rec.Body.String())
		}
	}
}

// TestHandlersSurfaceStoreFailures drives a read handler against a closed pool: a real
// driver error must become a 500 with a JSON body, not a panic or an empty 200.
func TestHandlersSurfaceStoreFailures(t *testing.T) {
	pool := setupClosedPool(t)
	svc := NewService(NewDBStore(pool))

	r := httptest.NewRequest(http.MethodGet, "/x?warehouse_id=00000000-0000-0000-0000-000000000001", nil)
	rec := httptest.NewRecorder()
	ListMovementsHandler(svc)(rec, r)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("movements with a dead pool = %d, want 500: %s", rec.Code, rec.Body.String())
	}
	// The writer's default arm says "server error"; what matters is a JSON body, not the text.
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body["error"] == nil {
		t.Fatalf("body = %s", rec.Body.String())
	}

	// The admin list writer must also answer JSON, never an empty body.
	admin := NewAdminStore(pool)
	r = httptest.NewRequest(http.MethodGet, "/x", nil)
	rec = httptest.NewRecorder()
	ListStockHandler(admin)(rec, r)
	if rec.Code != http.StatusInternalServerError || rec.Body.Len() == 0 {
		t.Fatalf("admin stock with a dead pool = %d, body %q", rec.Code, rec.Body.String())
	}
}

// TestGetStockBalanceHandler covers the balance read: a missing warehouse id is a 400 and an
// unknown balance is a 404.
func TestGetStockBalanceHandler(t *testing.T) {
	_, svc, _ := setupInventory(t)

	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	rec := httptest.NewRecorder()
	GetStockBalanceHandler(svc)(rec, r)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing warehouse id = %d, want 400: %s", rec.Code, rec.Body.String())
	}

	r = httptest.NewRequest(http.MethodGet, "/x", nil)
	r.SetPathValue("warehouseId", "00000000-0000-0000-0000-000000000001")
	r.SetPathValue("variantId", "00000000-0000-0000-0000-000000000002")
	rec = httptest.NewRecorder()
	GetStockBalanceHandler(svc)(rec, r)
	if rec.Code < 400 || rec.Code >= 500 {
		t.Fatalf("unknown balance = %d, want a 4xx", rec.Code)
	}
}

// setupClosedPool returns a pool that has been closed, so every query fails with a real
// driver error (pgx.ErrClosedPool) rather than a hand-made fake.
func setupClosedPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, _ := testpg.Setup(t)
	// Close immediately: the schema cleanup is skipped on purpose, since a closed pool cannot
	// drop it and the test only needs the driver error.
	pool.Close()
	return pool
}
