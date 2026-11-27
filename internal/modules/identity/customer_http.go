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

// addressPatch carries only the fields the caller sent. A nil pointer means "leave it
// alone", which is what PATCH means; the old handler required a full address and so behaved
// like PUT.
type addressPatch struct {
	Province       *string `json:"province"`
	City           *string `json:"city"`
	PostalCode     *string `json:"postal_code"`
	AddressLine    *string `json:"address_line"`
	Plaque         *string `json:"plaque"`
	Unit           *string `json:"unit"`
	RecipientName  *string `json:"recipient_name"`
	RecipientPhone *string `json:"recipient_phone"`
	IsDefault      *bool   `json:"is_default"`
}

// apply merges the patch into the stored address and validates the RESULT, so a partial
// update cannot leave a required field empty.
func (p addressPatch) apply(a Address) (Address, error) {
	if p.Province != nil {
		a.Province = *p.Province
	}
	if p.City != nil {
		a.City = *p.City
	}
	if p.PostalCode != nil {
		a.PostalCode = *p.PostalCode
	}
	if p.AddressLine != nil {
		a.AddressLine = *p.AddressLine
	}
	if p.Plaque != nil {
		a.Plaque = *p.Plaque
	}
	if p.Unit != nil {
		a.Unit = *p.Unit
	}
	if p.RecipientName != nil {
		a.RecipientName = *p.RecipientName
	}
	if p.RecipientPhone != nil {
		a.RecipientPhone = *p.RecipientPhone
	}
	if p.IsDefault != nil {
		a.IsDefault = *p.IsDefault
	}
	if a.Province == "" || a.City == "" || a.AddressLine == "" {
		return Address{}, errAddressIncomplete
	}
	return a, nil
}

var errAddressIncomplete = errors.New("identity: province, city and address_line are required")

// UpdateAddressHandler serves PATCH /api/v1/me/addresses/{id}: a partial update.
func UpdateAddressHandler(store CustomerStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		customerID, ok := requireCustomer(w, r)
		if !ok {
			return
		}
		var patch addressPatch
		if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		addressID := r.PathValue("id")
		// Read the stored row first: a partial update needs the fields it does not name.
		list, err := store.ListAddresses(r.Context(), customerID)
		if err != nil {
			writeCustomerErr(w, err)
			return
		}
		var current *Address
		for i := range list {
			if list[i].ID == addressID {
				current = &list[i]
				break
			}
		}
		if current == nil {
			writeErr(w, http.StatusNotFound, "address not found")
			return
		}
		merged, err := patch.apply(*current)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := store.UpdateAddress(r.Context(), customerID, addressID, merged); err != nil {
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
