package identity

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/khabavar/khabavar/internal/modules/authorization"
)

// MeHandler serves GET /api/v1/me.

// requireCustomer returns the authenticated customer id, or writes 401 and reports false.
// The route is behind AuthenticateCustomer, but a handler that trusts the context blindly
// turns a missing identity into a 500 from the store instead of a 401.
func requireCustomer(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := authorization.CustomerID(r.Context())
	if id == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{
			"error": "authentication required", "code": "unauthorized",
		})
		return "", false
	}
	return id, true
}

func MeHandler(store CustomerStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		customerID, ok := requireCustomer(w, r)
		if !ok {
			return
		}
		p, err := store.Profile(r.Context(), customerID)
		if err != nil {
			writeCustomerErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, p)
	}
}

// ListAddressesHandler serves GET /api/v1/me/addresses.
func ListAddressesHandler(store CustomerStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		customerID, ok := requireCustomer(w, r)
		if !ok {
			return
		}
		list, err := store.ListAddresses(r.Context(), customerID)
		if err != nil {
			writeCustomerErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": list})
	}
}

// CreateAddressHandler serves POST /api/v1/me/addresses.
func CreateAddressHandler(store CustomerStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		customerID, ok := requireCustomer(w, r)
		if !ok {
			return
		}
		a, ok := decodeAddress(w, r)
		if !ok {
			return
		}
		id, err := store.CreateAddress(r.Context(), customerID, a)
		if err != nil {
			writeCustomerErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"id": id})
	}
}

// UpdateAddressHandler serves PATCH /api/v1/me/addresses/{id}.
func UpdateAddressHandler(store CustomerStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		customerID, ok := requireCustomer(w, r)
		if !ok {
			return
		}
		a, ok := decodeAddress(w, r)
		if !ok {
			return
		}
		if err := store.UpdateAddress(r.Context(), customerID, r.PathValue("id"), a); err != nil {
			writeCustomerErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

// DeleteAddressHandler serves DELETE /api/v1/me/addresses/{id}.
func DeleteAddressHandler(store CustomerStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		customerID, ok := requireCustomer(w, r)
		if !ok {
			return
		}
		if err := store.DeleteAddress(r.Context(), customerID, r.PathValue("id")); err != nil {
			writeCustomerErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func decodeAddress(w http.ResponseWriter, r *http.Request) (Address, bool) {
	var a Address
	if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return Address{}, false
	}
	if a.Province == "" || a.City == "" || a.AddressLine == "" {
		writeErr(w, http.StatusBadRequest, "province, city and address_line are required")
		return Address{}, false
	}
	return a, true
}

func writeCustomerErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrCustomerNotFound):
		writeErr(w, http.StatusNotFound, "customer not found")
	case errors.Is(err, ErrAddressNotFound):
		writeErr(w, http.StatusNotFound, "address not found")
	default:
		writeErr(w, http.StatusInternalServerError, "internal error")
	}
}
