// Package notifications implements the notifications module (SAD §۴.۲, ADR-ARCH-011):
// SMS and email delivery adapters, plus a dispatcher that consumes domain events
// from the outbox and routes them to the right channel.
//
// The notification business rules:
//   - OTP codes are delivered ONLY via SMS (CIAM §۳-۴) — never email, never logged
//     in production. The SmsAdapter interface is the single delivery path.
//   - Order events (OrderPaid, OrderShipped, RefundIssued) are delivered via SMS
//     for customer status updates; email is optional (guest checkout has no email).
//   - The dispatcher is idempotent: it processes each event exactly once (the
//     outbox marks events as dispatched before the adapter runs; a retry re-delivers
//     only if the adapter reports failure).
//   - In production, SmsAdapter wraps the real SMS provider (Kavenegar/Farapayamak);
//     EmailAdapter wraps an SMTP/SES client. In dev/test, the mock adapters log.
package notifications

import (
	"context"
	"errors"
	"log/slog"
	"sync"
)

// Channel enums.
const (
	ChannelSMS   = "sms"
	ChannelEmail = "email"
)

// Sentinel errors.
var (
	ErrPhoneRequired   = errors.New("notifications: phone number required")
	ErrMessageRequired = errors.New("notifications: message required")
	ErrEmailRequired   = errors.New("notifications: email address required")
)

// SmsAdapter is the SMS delivery boundary. The real implementation wraps the
// SMS provider API; the mock logs the code length (never the raw code, CIAM §۳-۴).
type SmsAdapter interface {
	Send(ctx context.Context, phone, message string) error
}

// EmailAdapter is the email delivery boundary.
type EmailAdapter interface {
	Send(ctx context.Context, to, subject, body string) error
}

// MockSmsAdapter logs deliveries at debug level (never the raw OTP code).
type MockSmsAdapter struct {
	Logger *slog.Logger
	mu     sync.Mutex
	Sent   []MockDelivery // captured for test assertion
}

// MockDelivery records a mock SMS send for test inspection.
type MockDelivery struct {
	Phone  string
	MsgLen int
}

// Send logs the delivery and captures it. The raw message is NOT logged in
// production (CIAM §۳-۴); only its length.
func (m *MockSmsAdapter) Send(_ context.Context, phone, message string) error {
	if phone == "" {
		return ErrPhoneRequired
	}
	if message == "" {
		return ErrMessageRequired
	}
	m.mu.Lock()
	m.Sent = append(m.Sent, MockDelivery{Phone: phone, MsgLen: len(message)})
	m.mu.Unlock()
	if m.Logger != nil {
		m.Logger.Debug("sms mock deliver", "phone", phone, "msg_len", len(message))
	}
	return nil
}

// MockEmailAdapter logs email deliveries.
type MockEmailAdapter struct {
	Logger *slog.Logger
	mu     sync.Mutex
	Sent   []MockEmail
}

// MockEmail records a mock email send.
type MockEmail struct {
	To      string
	Subject string
	BodyLen int
}

func (m *MockEmailAdapter) Send(_ context.Context, to, subject, body string) error {
	if to == "" {
		return ErrEmailRequired
	}
	if subject == "" {
		return ErrMessageRequired
	}
	m.mu.Lock()
	m.Sent = append(m.Sent, MockEmail{To: to, Subject: subject, BodyLen: len(body)})
	m.mu.Unlock()
	if m.Logger != nil {
		m.Logger.Debug("email mock deliver", "to", to, "subject", subject, "body_len", len(body))
	}
	return nil
}

// Dispatcher routes a domain event to the right notification channel. It is the
// handler registered with the worker's outbox dispatcher (main.go runOutboxDispatcher).
type Dispatcher struct {
	sms   SmsAdapter
	email EmailAdapter
}

// NewDispatcher wires the SMS and email adapters.
func NewDispatcher(sms SmsAdapter, email EmailAdapter) *Dispatcher {
	return &Dispatcher{sms: sms, email: email}
}

// Event is the minimal event shape the dispatcher needs (mirrors worker.Event).
type Event struct {
	EventType   string
	AggregateID string
	Payload     map[string]any
}

// Dispatch routes an event to the notification channel based on its type.
//   - OrderPaid → SMS to customer phone ("سفارش شما ثبت شد")
//   - OrderShipped → SMS ("سفارش شما ارسال شد")
//   - RefundIssued → SMS ("بازپرداخت شما انجام شد")
//   - OtpCode → SMS (the OTP code; never logged)
//
// Unknown event types are silently skipped (the worker's fallback handles logging).
func (d *Dispatcher) Dispatch(ctx context.Context, e Event) error {
	phone, _ := e.Payload["phone"].(string)
	switch e.EventType {
	case "OrderPaid", "OrderShipped", "RefundIssued", "OrderConfirmed", "OrderCancelled":
		if phone == "" {
			return nil // guest without phone, or event lacks phone — skip silently
		}
		msg := defaultMessage(e.EventType, e.AggregateID)
		return d.sms.Send(ctx, phone, msg)
	case "OtpCode":
		code, _ := e.Payload["code"].(string)
		if phone == "" || code == "" {
			return nil
		}
		return d.sms.Send(ctx, phone, "کد تأیید شما: "+code)
	default:
		return nil
	}
}

// defaultMessage builds a human-readable Persian SMS for a customer event.
func defaultMessage(eventType, orderRef string) string {
	switch eventType {
	case "OrderPaid":
		return "سفارش شما " + orderRef + " با موفقیت پرداخت شد."
	case "OrderShipped":
		return "سفارش شما " + orderRef + " ارسال شد."
	case "RefundIssued":
		return "بازپرداخت سفارش " + orderRef + " انجام شد."
	case "OrderConfirmed":
		return "سفارش شما " + orderRef + " تأیید شد."
	case "OrderCancelled":
		return "سفارش " + orderRef + " لغو شد."
	default:
		return "وضعیت سفارش " + orderRef + " به‌روزرسانی شد."
	}
}

// SentCount returns the number of SMS deliveries captured by the mock (tests).
func (m *MockSmsAdapter) SentCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.Sent)
}
