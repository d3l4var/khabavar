// Package identity implements the identity module (SAD §۴.۱): OTP login/sessions,
// OIDC clients, OTP challenges. Entities: customers, customer_addresses, otp_challenges,
// identity_sessions, oauth_clients, oauth_authorization_codes. Dependency: shared.
//
// Flow F1 — OTP login (SAD §۷): phone → OTP challenge (hashed, attempt-capped,
// expiring) → verify → per-service session. Security events go to Audit.
package identity

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
)

// GenerateOTP returns a cryptographically random 6-digit numeric code (IAM §۵).
func GenerateOTP() (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("otp rand: %w", err)
	}
	n := uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
	return fmt.Sprintf("%06d", n%1000000), nil
}

// HashOTP returns a keyed HMAC-SHA256 hex digest of the code.
// The raw code is NEVER stored — only this hash (CIAM §۳-۴, otp_challenges.code_hash).
func HashOTP(code, serverKey string) string {
	mac := hmac.New(sha256.New, []byte(serverKey))
	mac.Write([]byte(code))
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyOTP compares a submitted code against the stored hash in constant time.
func VerifyOTP(submittedCode, storedHash, serverKey string) bool {
	want := HashOTP(submittedCode, serverKey)
	return subtle.ConstantTimeCompare([]byte(want), []byte(storedHash)) == 1
}

// randomToken returns a 32-byte hex token for session access tokens.
func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("token rand: %w", err)
	}
	return hex.EncodeToString(b), nil
}
