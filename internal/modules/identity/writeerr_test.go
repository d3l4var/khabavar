package identity

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestWriteErrCarriesACode pins the envelope: identity answered only {"error"}, so a client
// that branches on `code` — as the rest of the API teaches it to — got nothing here.
func TestWriteErrCarriesACode(t *testing.T) {
	cases := []struct{ msg, want string }{
		{"email and password required", "email_and_password_required"},
		{"invalid credentials", "invalid_credentials"},
		{"phone required", "phone_required"},
		{"invalid body", "invalid_body"},
		{"too many attempts", "too_many_attempts"},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		writeErr(rec, http.StatusBadRequest, c.msg)
		var body map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("%q: body is not JSON: %q", c.msg, rec.Body.String())
		}
		if body["code"] != c.want {
			t.Fatalf("%q: code = %q, want %q", c.msg, body["code"], c.want)
		}
		if body["error"] != c.msg {
			t.Fatalf("%q: error text changed to %q", c.msg, body["error"])
		}
	}
}

// TestMachineCodeIsStable covers the edge shapes: punctuation, digits, repeated separators.
func TestMachineCodeIsStable(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"Invalid  Body!!", "invalid_body"},
		{"  leading and trailing  ", "leading_and_trailing"},
		{"code 429", "code_429"},
		{"مرجوعی", ""},
	}
	for _, c := range cases {
		if got := machineCode(c.in); got != c.want {
			t.Fatalf("machineCode(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
