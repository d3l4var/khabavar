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
func OTPStartHandler(svc *Service, sms SMSProvider) http.HandlerFunc {
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
		// The purpose is a closed set (otp_challenges.purpose CHECK). Passing the raw string
		// through meant "login" — the obvious thing for a client to send — reached an INSERT and
		// answered 500. Validate here, default when omitted.
		purpose, ok := normalizePurpose(req.Purpose)
		if !ok {
			writeErr(w, http.StatusBadRequest, "invalid_purpose")
			return
		}
		code, err := svc.OTPStart(r.Context(), req.Phone, purpose, clientIP(r))
		if err != nil {
			if errors.Is(err, ErrRateLimited) {
				writeErr(w, http.StatusTooManyRequests, "rate limited")
				return
			}
			writeErr(w, http.StatusInternalServerError, "start failed")
			return
		}
		// A code the customer never receives is not "sent". Answering 202 when delivery failed
		// left the user waiting for an SMS that was never going to arrive, with no way to retry.
		// The OTP row already exists either way, so an error here reveals nothing about the phone.
		if sms != nil {
			if err := sms.Send(r.Context(), req.Phone, code); err != nil {
				writeErr(w, http.StatusServiceUnavailable, "sms_unavailable")
				return
			}
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"status": "sent"})
	}
}

// normalizePurpose accepts the canonical value and the lowercase spelling a client is likely to
// send, and treats an omitted purpose as a login. Anything else is a 400, not a 500.
// NormalizePurpose is exported for the handler tests in the external package.
func NormalizePurpose(raw string) (string, bool) { return normalizePurpose(raw) }

func normalizePurpose(raw string) (string, bool) {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "", PurposeLogin:
		return PurposeLogin, true
	case PurposeVerifyPhone:
		return PurposeVerifyPhone, true
	case PurposeAdminLogin:
		return PurposeAdminLogin, true
	default:
		return "", false
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
		purpose, ok := normalizePurpose(req.Purpose)
		if !ok {
			writeErr(w, http.StatusBadRequest, "invalid_purpose")
			return
		}
		customerID, err := svc.OTPVerify(r.Context(), req.Phone, purpose, req.Code)
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

// writeErr answers the same {"code","error"} envelope as every other module. It used to send
// only {"error"}, so a client that branches on `code` — which is what the rest of the API
// teaches it to do — got nothing from the identity endpoints.
func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{"error": msg, "code": machineCode(msg)})
}

// machineCode turns a human message into a stable snake_case identifier:
// "email and password required" -> "email_and_password_required".
func machineCode(msg string) string {
	var b strings.Builder
	prevUnderscore := true
	for _, r := range strings.ToLower(msg) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevUnderscore = false
		default:
			if !prevUnderscore {
				b.WriteByte('_')
				prevUnderscore = true
			}
		}
	}
	return strings.Trim(b.String(), "_")
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
