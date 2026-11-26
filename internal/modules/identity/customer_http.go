package identity

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/khabavar/khabavar/internal/modules/authorization"
)

// MeHandler serves GET /api/v1/me.
func MeHandler(store CustomerStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := store.Profile(r.Context(), authorization.CustomerID(r.Context()))
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
		list, err := store.ListAddresses(r.Context(), authorization.CustomerID(r.Context()))
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
		a, ok := decodeAddress(w, r)
		if !ok {
			return
		}
		id, err := store.CreateAddress(r.Context(), authorization.CustomerID(r.Context()), a)
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
		a, ok := decodeAddress(w, r)
		if !ok {
			return
		}
		if err := store.UpdateAddress(r.Context(), authorization.CustomerID(r.Context()), r.PathValue("id"), a); err != nil {
			writeCustomerErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

// DeleteAddressHandler serves DELETE /api/v1/me/addresses/{id}.
func DeleteAddressHandler(store CustomerStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := store.DeleteAddress(r.Context(), authorization.CustomerID(r.Context()), r.PathValue("id")); err != nil {
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
