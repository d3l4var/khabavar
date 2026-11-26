package commerce

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

// TestVerifyWebhookSignature pins the trust model: the signature covers the raw body, a
// wrong key or a tampered body is rejected, and an empty secret disables the webhook
// (fail closed).
func TestVerifyWebhookSignature(t *testing.T) {
	secret := PaymentWebhookSecret("s3cret")
	body := []byte(`{"provider":"mock","provider_reference":"R1","amount":1000,"status":"OK"}`)

	sign := func(key string, b []byte) string {
		mac := hmac.New(sha256.New, []byte(key))
		mac.Write(b)
		return hex.EncodeToString(mac.Sum(nil))
	}

	if !VerifyWebhookSignature(secret, body, sign("s3cret", body)) {
		t.Fatal("a correctly signed body must verify")
	}
	if !VerifyWebhookSignature(secret, body, sign("s3cret", body)+"\n") {
		t.Fatal("surrounding whitespace in the header must be tolerated")
	}
	if VerifyWebhookSignature(secret, body, sign("wrong", body)) {
		t.Fatal("a signature from another key must not verify")
	}
	if VerifyWebhookSignature(secret, []byte(`{"amount":9999}`), sign("s3cret", body)) {
		t.Fatal("a tampered body must not verify")
	}
	if VerifyWebhookSignature("", body, sign("", body)) {
		t.Fatal("an empty secret must fail closed")
	}
	if VerifyWebhookSignature(secret, body, "") {
		t.Fatal("a missing signature must fail")
	}
}
