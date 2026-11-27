package commerce

import (
	"errors"
	"fmt"
	"testing"
)

// TestErrorCodeIsMachineReadable pins the code a client branches on. It used to be derived
// from the message by cutting at the first ':', and every message starts with "commerce:",
// so every error answered {"code":"COMMERCE"}.
func TestErrorCodeIsMachineReadable(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{ErrCartNotFound, "cart_not_found"},
		{ErrCartTokenRequired, "cart_token_required"},
		{ErrCartEmpty, "cart_empty"},
		{ErrCustomerSessionRequired, "customer_session_required"},
		{ErrVariantRequired, "variant_required"},
		{ErrShippingAddressReq, "shipping_address_required"},
		{ErrGuestPhoneRequired, "guest_phone_required"},
		{ErrIdempotencyKeyMissing, "idempotency_key_required"},
		{ErrOrderNotFound, "order_not_found"},
		{ErrIntentExists, "payment_intent_exists"},
		{ErrAlreadyPaid, "already_paid"},
		{ErrRefundExceedsPaid, "refund_exceeds_paid"},
		{ErrOrderNotRefundable, "order_not_refundable"},
		{ErrReturnReasonRequired, "return_reason_required"},
		// A wrapped sentinel still maps: the detail suffix must not change the code.
		{fmt.Errorf("%w: status=CLOSED", ErrOrderNotRefundable), "order_not_refundable"},
		{fmt.Errorf("%w: amount=5 available=1", ErrRefundExceedsPaid), "refund_exceeds_paid"},
		// An unknown error gets a generic code, not a misleading one.
		{errors.New("commerce: something new"), "request_failed"},
	}
	for _, c := range cases {
		if got := errorCode(c.err); got != c.want {
			t.Fatalf("errorCode(%v) = %q, want %q", c.err, got, c.want)
		}
	}
	// No code is the bare package name any more.
	for _, c := range cases {
		if errorCode(c.err) == "COMMERCE" {
			t.Fatalf("%v still yields the package name as its code", c.err)
		}
	}
}
