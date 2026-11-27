package identity

import "context"

// captureSMS records the code a provider was asked to send, for tests that need the value.
type captureSMS struct{ into *struct{ phone, code string } }

func (c captureSMS) Name() string { return "capture" }
func (c captureSMS) Send(_ context.Context, phone, code string) error {
	c.into.phone, c.into.code = phone, code
	return nil
}
