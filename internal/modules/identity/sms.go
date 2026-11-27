package identity

import (
	"context"
	"log/slog"
)

// SMSProvider delivers a one-time code to a phone number.
//
// Why an interface: the OTP handler used to take a bare func(phone, code string), so there was
// nowhere to put a real provider — no place for a context, no way to report a delivery failure,
// and no name to put in a log line. Payment and object storage already have adapter seams; SMS
// was the one that did not.
//
// A real implementation (Kavenegar, Melipayamak, sms.ir) implements this and is selected by
// KH_SMS_ADAPTER. Nothing else in the codebase changes.
type SMSProvider interface {
	// Send delivers code to phone. An error means the customer did not receive it.
	Send(ctx context.Context, phone, code string) error
	// Name identifies the adapter in logs and in the startup banner.
	Name() string
}

// MockSMS is the development adapter: it logs that a code would have been sent and never fails.
// It is the default so a fresh checkout runs without credentials.
type MockSMS struct {
	Logger *slog.Logger
}

func (m MockSMS) Name() string { return "mock" }

func (m MockSMS) Send(_ context.Context, phone, code string) error {
	// A nil logger used to panic here, which killed the request and left the client with no
	// response at all. A mock sink must never be able to do that.
	if m.Logger == nil {
		return nil
	}
	m.Logger.Debug("sms mock deliver", "phone", phone, "code_len", len(code))
	return nil
}
