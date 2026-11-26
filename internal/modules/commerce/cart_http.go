package commerce

import (
	"encoding/json"
	"errors"
	"net/http"
)

// guestTokenHeader identifies a guest cart (contract: X-Guest-Cart-Token).
const guestTokenHeader = "X-Guest-Cart-Token"

// CartHandler serves GET /api/v1/cart.
func CartHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		view, err := svc.Cart(r.Context(), "", r.Header.Get(guestTokenHeader))
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, view)
	}
}

// AddCartItemHandler serves POST /api/v1/cart/items.
func AddCartItemHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			VariantID string `json:"variant_id"`
			Quantity  int    `json:"quantity"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json", "code": "invalid_body"})
			return
		}
		view, err := svc.AddToCart(r.Context(), "", r.Header.Get(guestTokenHeader), req.VariantID, req.Quantity)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, view)
	}
}

// UpdateCartItemHandler serves PATCH /api/v1/cart/items/{id} (quantity 0 removes).
func UpdateCartItemHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Quantity int `json:"quantity"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json", "code": "invalid_body"})
			return
		}
		view, err := svc.UpdateCartItem(r.Context(), "", r.Header.Get(guestTokenHeader), r.PathValue("id"), req.Quantity)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, view)
	}
}

// DeleteCartItemHandler serves DELETE /api/v1/cart/items/{id}.
func DeleteCartItemHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		view, err := svc.RemoveCartItem(r.Context(), "", r.Header.Get(guestTokenHeader), r.PathValue("id"))
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, view)
	}
}

// cartErrorCode maps cart errors to the contract's codes and statuses.
func cartErrorCode(err error) (int, string) {
	switch {
	case errors.Is(err, ErrCartTokenRequired):
		return http.StatusBadRequest, "cart_token_required"
	case errors.Is(err, ErrShippingAddressReq):
		return http.StatusBadRequest, "shipping_address_required"
	case errors.Is(err, ErrGuestPhoneRequired):
		return http.StatusBadRequest, "guest_phone_required"
	case errors.Is(err, ErrCartNotFound):
		return http.StatusNotFound, "cart_not_found"
	case errors.Is(err, ErrCartItemNotFound):
		return http.StatusNotFound, "cart_item_not_found"
	case errors.Is(err, ErrCartQuantityInvalid):
		return http.StatusBadRequest, "invalid_quantity"
	default:
		return 0, ""
	}
}

// CheckoutHandler serves POST /api/v1/checkout (contract: order + payment intent).
func CheckoutHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in CheckoutInput
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json", "code": "invalid_body"})
			return
		}
		res, err := svc.Checkout(r.Context(), "", r.Header.Get(guestTokenHeader), in)
		if err != nil {
			var unavailable *ErrCheckoutUnavailable
			if errors.As(err, &unavailable) {
				writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
					"error": "some lines are not sellable right now",
					"code":  "checkout_unavailable",
					"items": unavailable.Lines,
				})
				return
			}
			if errors.Is(err, ErrCartEmpty) {
				writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "code": "cart_empty"})
				return
			}
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, res)
	}
}
