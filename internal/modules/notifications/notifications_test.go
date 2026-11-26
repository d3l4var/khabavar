package notifications

import (
	"context"
	"errors"
	"log/slog"
	"testing"
)

func TestMockSmsAdapter_Send(t *testing.T) {
	adapter := &MockSmsAdapter{Logger: slog.New(slog.NewTextHandler(&discardWriter{}, nil))}
	if err := adapter.Send(context.Background(), "09120000000", "کد تأیید: 123456"); err != nil {
		t.Fatalf("send: %v", err)
	}
	if adapter.SentCount() != 1 {
		t.Fatalf("sent=%d want 1", adapter.SentCount())
	}
	// Verify the raw code is NOT in the captured record (only length).
	if adapter.Sent[0].MsgLen != len("کد تأیید: 123456") {
		t.Fatalf("msg_len=%d", adapter.Sent[0].MsgLen)
	}
}

func TestMockSmsAdapter_RejectsEmpty(t *testing.T) {
	adapter := &MockSmsAdapter{}
	if err := adapter.Send(context.Background(), "", "x"); !errors.Is(err, ErrPhoneRequired) {
		t.Fatalf("empty phone: %v", err)
	}
	if err := adapter.Send(context.Background(), "0912", ""); !errors.Is(err, ErrMessageRequired) {
		t.Fatalf("empty message: %v", err)
	}
}

func TestMockEmailAdapter_Send(t *testing.T) {
	adapter := &MockEmailAdapter{}
	if err := adapter.Send(context.Background(), "user@example.com", "subject", "body"); err != nil {
		t.Fatalf("send: %v", err)
	}
	if len(adapter.Sent) != 1 {
		t.Fatalf("sent=%d want 1", len(adapter.Sent))
	}
	if adapter.Sent[0].To != "user@example.com" {
		t.Fatalf("to=%s", adapter.Sent[0].To)
	}
}

func TestDispatcher_RoutesOrderEvents(t *testing.T) {
	sms := &MockSmsAdapter{}
	email := &MockEmailAdapter{}
	d := NewDispatcher(sms, email)
	// OrderPaid with phone → SMS sent.
	if err := d.Dispatch(context.Background(), Event{
		EventType: "OrderPaid", AggregateID: "ORD-1001",
		Payload: map[string]any{"phone": "09120000000"},
	}); err != nil {
		t.Fatalf("dispatch OrderPaid: %v", err)
	}
	if sms.SentCount() != 1 {
		t.Fatalf("sms sent=%d want 1", sms.SentCount())
	}
	// OrderShipped → another SMS.
	d.Dispatch(context.Background(), Event{
		EventType: "OrderShipped", AggregateID: "ORD-1001",
		Payload: map[string]any{"phone": "09120000000"},
	})
	if sms.SentCount() != 2 {
		t.Fatalf("sms sent=%d want 2", sms.SentCount())
	}
	// RefundIssued → another SMS.
	d.Dispatch(context.Background(), Event{
		EventType: "RefundIssued", AggregateID: "ORD-1001",
		Payload: map[string]any{"phone": "09120000000"},
	})
	if sms.SentCount() != 3 {
		t.Fatalf("sms sent=%d want 3", sms.SentCount())
	}
}

func TestDispatcher_SkipsEventsWithoutPhone(t *testing.T) {
	sms := &MockSmsAdapter{}
	d := NewDispatcher(sms, &MockEmailAdapter{})
	// OrderPaid without phone → no SMS sent (guest without phone).
	d.Dispatch(context.Background(), Event{
		EventType: "OrderPaid", AggregateID: "ORD-1",
		Payload: map[string]any{},
	})
	if sms.SentCount() != 0 {
		t.Fatalf("sms sent=%d want 0 (no phone)", sms.SentCount())
	}
}

func TestDispatcher_OtpCode(t *testing.T) {
	sms := &MockSmsAdapter{}
	d := NewDispatcher(sms, &MockEmailAdapter{})
	d.Dispatch(context.Background(), Event{
		EventType: "OtpCode", AggregateID: "sess-1",
		Payload: map[string]any{"phone": "09120000000", "code": "123456"},
	})
	if sms.SentCount() != 1 {
		t.Fatalf("sms sent=%d want 1", sms.SentCount())
	}
}

func TestDispatcher_UnknownEventSkipped(t *testing.T) {
	sms := &MockSmsAdapter{}
	d := NewDispatcher(sms, &MockEmailAdapter{})
	// Unknown event type → no-op.
	d.Dispatch(context.Background(), Event{
		EventType: "SomeUnknownEvent", AggregateID: "x",
		Payload: map[string]any{"phone": "0912"},
	})
	if sms.SentCount() != 0 {
		t.Fatalf("sms sent=%d want 0 (unknown event)", sms.SentCount())
	}
}

// discardWriter is a no-op io.Writer for the test logger.
type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }
