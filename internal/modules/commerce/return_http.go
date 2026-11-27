package commerce

import (
	"encoding/json"
	"errors"
	"net/http"
)

// Admin handlers for the return claim lifecycle. The customer can only create a claim
// (POST /orders/{id}/return-requests); every decision after that belongs to staff.

func returnErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrReturnNotFound):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: err.Error(), Code: "return_not_found"})
	case errors.Is(err, ErrReturnTransition):
		writeJSON(w, http.StatusConflict, errorResponse{Error: err.Error(), Code: "invalid_return_transition"})
	case errors.Is(err, ErrReturnDecisionNote):
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error(), Code: "decision_note_required"})
	default:
		writeError(w, err)
	}
}

type returnDecisionBody struct {
	Note string `json:"note"`
}

// ApproveReturnHandler serves POST /api/v1/admin/return-requests/{id}/approve.
func ApproveReturnHandler(svc *Service) http.HandlerFunc {
	return decideReturnHandler(svc, true)
}

// RejectReturnHandler serves POST /api/v1/admin/return-requests/{id}/reject — the note is
// mandatory: a customer whose return was refused is owed a reason.
func RejectReturnHandler(svc *Service) http.HandlerFunc {
	return decideReturnHandler(svc, false)
}

func decideReturnHandler(svc *Service, approve bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body returnDecisionBody
		if r.ContentLength > 0 {
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid json", Code: "invalid_body"})
				return
			}
		}
		rr, err := svc.DecideReturn(r.Context(), r.PathValue("id"), approve, body.Note)
		if err != nil {
			returnErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, rr)
	}
}

// ReceiveReturnHandler serves POST /api/v1/admin/return-requests/{id}/receive — the goods
// came back. It moves no money; the refund is a separate audited step.
func ReceiveReturnHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body returnDecisionBody
		if r.ContentLength > 0 {
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid json", Code: "invalid_body"})
				return
			}
		}
		rr, err := svc.ReceiveReturn(r.Context(), r.PathValue("id"), body.Note)
		if err != nil {
			returnErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, rr)
	}
}
