// Package inventory — HTTP handlers for inventory admin + storefront read endpoints (R-07 §inventory).
// Admin endpoints (create warehouse, receive/adjust stock, reserve/release/convert) are
// mounted behind authorization.Require in main.go; storefront reads (stock balance) are public.
package inventory

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/khabavar/khabavar/internal/modules/shared"
)

// CreateWarehouseHandler serves POST /api/v1/admin/warehouses.
func CreateWarehouseHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name          string `json:"name"`
			WarehouseCode string `json:"warehouse_code"`
			Address       string `json:"address"`
			Status        string `json:"status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeInvErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		if req.Status == "" {
			req.Status = "ACTIVE"
		}
		id, err := svc.CreateWarehouse(r.Context(), Warehouse{
			Name: req.Name, WarehouseCode: req.WarehouseCode, Address: req.Address, Status: req.Status,
		})
		if err != nil {
			writeInvSvcErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"id": id})
	}
}

// GetStockBalanceHandler serves GET /api/v1/warehouses/{wid}/variants/{vid}/stock —
// the storefront reads available stock (on_hand − reserved) for a variant (R-07).
func GetStockBalanceHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		wid := r.PathValue("wid")
		vid := r.PathValue("vid")
		if wid == "" || vid == "" {
			writeInvErr(w, http.StatusBadRequest, "warehouse and variant id required")
			return
		}
		sb, err := svc.GetStockBalance(r.Context(), wid, vid)
		if err != nil {
			writeInvSvcErr(w, err)
			return
		}
		if sb == nil {
			writeInvSvcErr(w, ErrStockBalanceNotFound)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": sb})
	}
}

// ReceiveStockHandler serves POST /api/v1/admin/warehouses/{wid}/variants/{vid}/receive.
func ReceiveStockHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		wid := r.PathValue("wid")
		vid := r.PathValue("vid")
		var req struct {
			Quantity      int    `json:"quantity"`
			ReferenceType string `json:"reference_type"`
			ReferenceID   string `json:"reference_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeInvErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		// The actor comes from the authenticated context, never from the body: a stock
		// movement must be attributable to the real caller (IAM §۷).
		actor := shared.ActorID(r.Context())
		if actor == "" {
			writeInvErr(w, http.StatusUnauthorized, "an authenticated actor is required")
			return
		}
		if err := svc.ReceiveStock(r.Context(), wid, vid, req.Quantity, req.ReferenceType, req.ReferenceID, actor); err != nil {
			writeInvSvcErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

// AdjustStockHandler serves POST /api/v1/admin/warehouses/{wid}/variants/{vid}/adjust.
func AdjustStockHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		wid := r.PathValue("wid")
		vid := r.PathValue("vid")
		var req struct {
			Delta         int    `json:"delta"`
			ReferenceType string `json:"reference_type"`
			ReferenceID   string `json:"reference_id"`
			Notes         string `json:"notes"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeInvErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		actor := shared.ActorID(r.Context())
		if actor == "" {
			writeInvErr(w, http.StatusUnauthorized, "an authenticated actor is required")
			return
		}
		if err := svc.AdjustStock(r.Context(), wid, vid, req.Delta, req.ReferenceType, req.ReferenceID, actor, req.Notes); err != nil {
			writeInvSvcErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

// ReserveForOrderHandler serves POST /api/v1/admin/inventory/reserve — reserve
// stock for an order item (called by commerce during checkout, §inventory reserve).
func ReserveForOrderHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			WarehouseID string `json:"warehouse_id"`
			SupplierID  string `json:"supplier_id"`
			VariantID   string `json:"variant_id"`
			OrderItemID string `json:"order_item_id"`
			Quantity    int    `json:"quantity"`
			ExpiresAt   string `json:"expires_at"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeInvErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		var expires time.Time
		if req.ExpiresAt != "" {
			if t, err := time.Parse(time.RFC3339, req.ExpiresAt); err == nil {
				expires = t
			}
		}
		id, err := svc.ReserveForOrder(r.Context(), Reservation{
			WarehouseID: req.WarehouseID, SupplierID: req.SupplierID,
			VariantID: req.VariantID, OrderItemID: req.OrderItemID,
			Quantity: req.Quantity, ExpiresAt: expires,
		})
		if err != nil {
			writeInvSvcErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"id": id})
	}
}

// ReleaseReservationHandler serves DELETE /api/v1/admin/inventory/reservations/{id}.
func ReleaseReservationHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if err := svc.ReleaseReservation(r.Context(), id); err != nil {
			writeInvSvcErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

// ConvertReservationHandler serves POST /api/v1/admin/inventory/reservations/{id}/convert —
// convert a reservation to a permanent stock deduction (on order payment, §inventory convert).
func ConvertReservationHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if err := svc.ConvertReservation(r.Context(), id); err != nil {
			writeInvSvcErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

// ListMovementsHandler serves GET /api/v1/admin/warehouses/{wid}/variants/{vid}/movements —
// the movement ledger (append-only audit trail, §inventory ledger).
func ListMovementsHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		wid := r.PathValue("wid")
		vid := r.PathValue("vid")
		limit := parsePositiveInt(r.URL.Query().Get("limit"), 50)
		movs, err := svc.ListMovements(r.Context(), wid, vid, limit)
		if err != nil {
			writeInvSvcErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": movs})
	}
}

func writeInvSvcErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrWarehouseNotFound), errors.Is(err, ErrStockBalanceNotFound), errors.Is(err, ErrReservationNotFound):
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
	case errors.Is(err, ErrInsufficientStock):
		writeJSON(w, http.StatusConflict, map[string]any{"error": "insufficient stock", "code": "insufficient_stock"})
	case errors.Is(err, ErrReservationNotActive):
		writeJSON(w, http.StatusConflict, map[string]any{"error": "reservation not active", "code": "reservation_not_active"})
	case errors.Is(err, ErrOrderItemRequired):
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "order_item_id is required", "code": "order_item_required"})
	case errors.Is(err, ErrReservationExpiryReq):
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "reservation expires_at required"})
	case errors.Is(err, ErrZeroQuantity):
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "quantity must be non-zero"})
	case errors.Is(err, ErrWarehouseCodeReq), errors.Is(err, ErrWarehouseNameReq):
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "warehouse name and code required"})
	case errors.Is(err, ErrDuplicateWarehouseCode):
		writeJSON(w, http.StatusConflict, map[string]any{"error": "warehouse_code already exists", "code": "duplicate_warehouse_code"})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "server error"})
	}
}

func writeInvErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{"error": msg})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func parsePositiveInt(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return def
	}
	return n
}
