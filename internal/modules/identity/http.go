package identity

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

type otpStartRequest struct {
	Phone   string `json:"phone"`
	Purpose string `json:"purpose"`
}

type otpVerifyRequest struct {
	Phone   string `json:"phone"`
	Purpose string `json:"purpose"`
	Code    string `json:"code"`
}

// OTPStartHandler initiates an OTP challenge (F1). The generated code is handed to
// `deliver` (the SmsAdapter — Mock in v1, ADR-ARCH-011); it is NEVER returned to the
// client (CIAM §۳-۴: raw code never leaves the server via response body).
func OTPStartHandler(svc *Service, deliver func(phone, code string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req otpStartRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid body")
			return
		}
		if req.Phone == "" {
			writeErr(w, http.StatusBadRequest, "phone required")
			return
		}
		code, err := svc.OTPStart(r.Context(), req.Phone, req.Purpose, clientIP(r))
		if err != nil {
			if errors.Is(err, ErrRateLimited) {
				writeErr(w, http.StatusTooManyRequests, "rate limited")
				return
			}
			writeErr(w, http.StatusInternalServerError, "start failed")
			return
		}
		if deliver != nil {
			deliver(req.Phone, code)
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"status": "sent"})
	}
}

// OTPVerifyHandler verifies the OTP and, on success, issues a per-service session
// token. serviceCode is taken from the X-Khabavar-Service header (default "shop").
func OTPVerifyHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req otpVerifyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid body")
			return
		}
		if req.Phone == "" || req.Code == "" {
			writeErr(w, http.StatusBadRequest, "phone and code required")
			return
		}
		customerID, err := svc.OTPVerify(r.Context(), req.Phone, req.Purpose, req.Code)
		if err != nil {
			writeVerifyErr(w, err)
			return
		}
		if customerID == "" {
			writeErr(w, http.StatusUnauthorized, "no customer")
			return
		}
		service := r.Header.Get("X-Khabavar-Service")
		if service == "" {
			service = "shop"
		}
		token, err := svc.IssueSession(r.Context(), customerID, service, clientIP(r), r.UserAgent())
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "session failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"access_token": token, "service": service})
	}
}

func writeVerifyErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidCode), errors.Is(err, ErrChallengeNone), errors.Is(err, ErrAlreadyConsumed):
		writeErr(w, http.StatusUnauthorized, "invalid code")
	case errors.Is(err, ErrChallengeExpired), errors.Is(err, ErrMaxAttempts):
		writeErr(w, http.StatusGone, "challenge expired")
	default:
		writeErr(w, http.StatusInternalServerError, "verify failed")
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{"error": msg})
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.Index(xff, ","); i > 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	host := r.RemoteAddr
	if i := strings.LastIndex(host, ":"); i > 0 {
		host = host[:i]
	}
	return host
}
