// Package fulfillment — HTTP handlers for fulfillment admin + storefront read endpoints (R-07 §fulfillment).
// Admin endpoints (create fulfillment, status transitions, shipments) are mounted behind
// authorization.Require in main.go; storefront reads (fulfillment status by order) are public.
package fulfillment

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"
)

// CreateFulfillmentOrderHandler serves POST /api/v1/admin/fulfillment-orders.
func CreateFulfillmentOrderHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			OrderID     string `json:"order_id"`
			SourceType  string `json:"source_type"`
			WarehouseID string `json:"warehouse_id"`
			SupplierID  string `json:"supplier_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeFulErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		id, err := svc.CreateFulfillmentOrder(r.Context(), FulfillmentOrder{
			OrderID: req.OrderID, SourceType: req.SourceType,
			WarehouseID: req.WarehouseID, SupplierID: req.SupplierID,
		})
		if err != nil {
			writeFulSvcErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"id": id})
	}
}

// transitionHandler is the shared builder for the PENDING→CONFIRMED etc. transitions.
func transitionHandler(svc *Service, fn func(context.Context, string) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if err := fn(r.Context(), id); err != nil {
			writeFulSvcErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

// ConfirmFulfillmentHandler serves POST /api/v1/admin/fulfillment-orders/{id}/confirm.
func ConfirmFulfillmentHandler(svc *Service) http.HandlerFunc {
	return transitionHandler(svc, svc.ConfirmFulfillmentOrder)
}

// StartProcessingHandler serves POST /api/v1/admin/fulfillment-orders/{id}/processing.
func StartProcessingHandler(svc *Service) http.HandlerFunc {
	return transitionHandler(svc, svc.StartProcessing)
}

// MarkReadyHandler serves POST /api/v1/admin/fulfillment-orders/{id}/ready.
func MarkReadyHandler(svc *Service) http.HandlerFunc {
	return transitionHandler(svc, svc.MarkReady)
}

// MarkShippedHandler serves POST /api/v1/admin/fulfillment-orders/{id}/shipped.
func MarkShippedHandler(svc *Service) http.HandlerFunc {
	return transitionHandler(svc, svc.MarkShipped)
}

// MarkDeliveredHandler serves POST /api/v1/admin/fulfillment-orders/{id}/delivered.
func MarkDeliveredHandler(svc *Service) http.HandlerFunc {
	return transitionHandler(svc, svc.MarkDelivered)
}

// CancelFulfillmentHandler serves POST /api/v1/admin/fulfillment-orders/{id}/cancel.
func CancelFulfillmentHandler(svc *Service) http.HandlerFunc {
	return transitionHandler(svc, svc.CancelFulfillmentOrder)
}

// RejectFulfillmentHandler serves POST /api/v1/admin/fulfillment-orders/{id}/reject.
func RejectFulfillmentHandler(svc *Service) http.HandlerFunc {
	return transitionHandler(svc, svc.RejectFulfillmentOrder)
}

// CreateShipmentHandler serves POST /api/v1/admin/fulfillment-orders/{id}/shipments.
func CreateShipmentHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		foID := r.PathValue("id")
		var req struct {
			Carrier        string `json:"carrier"`
			ServiceType    string `json:"service_type"`
			TrackingNumber string `json:"tracking_number"`
			ShippingCost   int64  `json:"shipping_cost"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeFulErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		id, err := svc.CreateShipment(r.Context(), Shipment{
			FulfillmentOrderID: foID, Carrier: req.Carrier, ServiceType: req.ServiceType,
			TrackingNumber: req.TrackingNumber, ShippingCost: req.ShippingCost,
		})
		if err != nil {
			writeFulSvcErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"id": id})
	}
}

// TransitionShipmentHandler serves POST /api/v1/admin/shipments/{id}/transition.
func TransitionShipmentHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		shID := r.PathValue("id")
		var req struct {
			To          string `json:"to"`
			EventCode   string `json:"event_code"`
			Description string `json:"description"`
			Source      string `json:"source"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeFulErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		if req.Source == "" {
			req.Source = EventSourceManual
		}
		if err := svc.TransitionShipmentStatus(r.Context(), shID, req.To, req.EventCode, req.Description, req.Source); err != nil {
			writeFulSvcErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

// ListShipmentEventsHandler serves GET /api/v1/shipments/{id}/events — the append-only
// tracking timeline (§11.5). Public so the storefront can show tracking history.
func ListShipmentEventsHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		shID := r.PathValue("id")
		evs, err := svc.ListShipmentEvents(r.Context(), shID)
		if err != nil {
			writeFulSvcErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": evs})
	}
}

// ListFulfillmentOrdersByOrderHandler serves GET /api/v1/orders/{id}/fulfillment —
// the storefront reads the fulfillment status for a customer's order (R-07).
func ListFulfillmentOrdersByOrderHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orderID := r.PathValue("id")
		fos, err := svc.ListFulfillmentOrdersByOrder(r.Context(), orderID)
		if err != nil {
			writeFulSvcErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": fos})
	}
}

// ListShipmentsByFulfillmentHandler serves GET /api/v1/fulfillment-orders/{id}/shipments.
func ListShipmentsByFulfillmentHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		foID := r.PathValue("id")
		shs, err := svc.ListShipmentsByFulfillment(r.Context(), foID)
		if err != nil {
			writeFulSvcErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": shs})
	}
}

func writeFulSvcErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrFulfillmentNotFound), errors.Is(err, ErrShipmentNotFound), errors.Is(err, ErrFulfillmentItemNotFound):
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
	case errors.Is(err, ErrSourceTypeMismatch):
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "source_type and source reference mismatch", "code": "source_mismatch"})
	case errors.Is(err, ErrInvalidFulfillmentStatus), errors.Is(err, ErrInvalidShipmentStatus):
		writeJSON(w, http.StatusConflict, map[string]any{"error": "invalid status transition", "code": "invalid_status"})
	case errors.Is(err, ErrAllocationExceedsOrder):
		writeJSON(w, http.StatusConflict, map[string]any{"error": "allocation exceeds order item quantity", "code": "allocation_exceeds"})
	case errors.Is(err, ErrQuantityMustBePositive):
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "quantity must be positive"})
	case errors.Is(err, ErrOrderIDRequired):
		// A missing order id is the client's mistake: without this branch it fell to the
		// default 500, which is what the handler test caught.
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "order_id is required", "code": "order_id_required"})
	case errors.Is(err, ErrInvalidEventSource):
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "event source must be MANUAL, CARRIER_API or WEBHOOK", "code": "invalid_event_source"})
	case errors.Is(err, ErrCarrierRequired):
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "carrier required"})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "server error"})
	}
}

func writeFulErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{"error": msg})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// RecordShipmentEventHandler serves POST /api/v1/admin/shipments/{id}/events — a tracking
// update that does not change the shipment status.
func RecordShipmentEventHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			EventCode   string `json:"event_code"`
			Description string `json:"description"`
			Source      string `json:"source"`
			OccurredAt  string `json:"occurred_at"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeFulErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		var at time.Time
		if req.OccurredAt != "" {
			parsed, err := time.Parse(time.RFC3339, req.OccurredAt)
			if err != nil {
				writeFulErr(w, http.StatusBadRequest, "occurred_at must be RFC3339")
				return
			}
			at = parsed
		}
		id, err := svc.AddShipmentEvent(r.Context(), r.PathValue("id"), req.EventCode, req.Description, req.Source, at)
		if err != nil {
			writeFulSvcErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"id": id})
	}
}

// CreateOrderFulfillmentHandler serves POST /api/v1/admin/orders/{id}/fulfillments.
// The order id comes from the path and is authoritative: a body order_id that disagrees
// is rejected rather than silently ignored, so a client cannot fulfil the wrong order.
func CreateOrderFulfillmentHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			OrderID     string `json:"order_id"`
			SourceType  string `json:"source_type"`
			WarehouseID string `json:"warehouse_id"`
			SupplierID  string `json:"supplier_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
			writeFulErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		orderID := r.PathValue("id")
		if req.OrderID != "" && req.OrderID != orderID {
			writeFulErr(w, http.StatusBadRequest, "order_id in the body contradicts the path")
			return
		}
		id, err := svc.CreateFulfillmentOrder(r.Context(), FulfillmentOrder{
			OrderID: orderID, SourceType: req.SourceType,
			WarehouseID: req.WarehouseID, SupplierID: req.SupplierID,
		})
		if err != nil {
			writeFulSvcErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"id": id, "order_id": orderID})
	}
}
