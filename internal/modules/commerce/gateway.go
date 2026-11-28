package commerce

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// PaymentGateway starts a payment at a real provider and confirms what the customer actually paid.
//
// Why this exists: KH_PAYMENT_ADAPTER only stamped a name onto a payment attempt. Nothing ever
// talked to a gateway, so the customer was never sent anywhere to pay — the intent existed and
// the order stayed PENDING_PAYMENT forever. A mock-first design still needs the seam, or the
// first real integration is a rewrite.
type PaymentGateway interface {
	// Start opens a transaction and returns the URL the customer must be sent to.
	Start(ctx context.Context, req StartPayment) (StartResult, error)
	// Verify confirms the amount paid for a provider reference. It must never trust the
	// amount the callback claims: the gateway is asked, and the answer is compared.
	Verify(ctx context.Context, req VerifyPayment) (VerifyResult, error)
	// Name identifies the adapter in logs and on the payment attempt row.
	Name() string
}

// StartPayment is what a gateway needs to open a transaction.
type StartPayment struct {
	OrderID     string
	Amount      int64 // IRR, integer (ADR-DATA-001)
	Description string
	CallbackURL string
	// Mobile is optional; some gateways prefill it.
	Mobile string
}

// StartResult is the redirect the customer follows.
type StartResult struct {
	Authority   string // the provider's transaction handle
	RedirectURL string
}

// VerifyPayment asks the gateway about a reference.
type VerifyPayment struct {
	Authority string
	Amount    int64
}

// VerifyResult is the gateway's verdict.
type VerifyResult struct {
	Paid      bool
	Reference string // the provider's settlement reference (for the audit trail)
	Amount    int64
}

// ErrGatewayRejected is returned when the provider answers with a business error. It is a
// distinct sentinel so the handler can answer 409 rather than 500.
var ErrGatewayRejected = errors.New("commerce: payment gateway rejected the request")

// MockGateway is the development adapter: it hands back a local URL and always verifies true.
// It is the default so a fresh checkout completes a purchase without credentials.
type MockGateway struct {
	// BaseURL is where the mock "gateway" page lives; the customer is sent here.
	BaseURL string
}

func (m MockGateway) Name() string { return "mock" }

func (m MockGateway) Start(_ context.Context, req StartPayment) (StartResult, error) {
	auth := "MOCK-" + req.OrderID
	base := m.BaseURL
	if base == "" {
		base = "/mock-payment"
	}
	return StartResult{
		Authority:   auth,
		RedirectURL: fmt.Sprintf("%s?authority=%s&amount=%d", base, auth, req.Amount),
	}, nil
}

func (m MockGateway) Verify(_ context.Context, req VerifyPayment) (VerifyResult, error) {
	return VerifyResult{Paid: true, Reference: "MOCK-REF-" + req.Authority, Amount: req.Amount}, nil
}

// ZarinpalGateway talks to Zarinpal's REST API (v4).
//
// The shape here — POST JSON, a numeric code where 100 means success, an authority string, then
// a separate verify call — is the same shape IDPay, Zibal and NextPay use, so a second provider
// is a new file rather than a new design.
type ZarinpalGateway struct {
	BaseURL     string // https://payment.zarinpal.com/pg/v4/payment
	StartURL    string // https://payment.zarinpal.com/pg/StartPay/
	MerchantID  string
	CallbackURL string
	Client      *http.Client
}

func (z ZarinpalGateway) Name() string { return "zarinpal" }

func (z ZarinpalGateway) client() *http.Client {
	if z.Client != nil {
		return z.Client
	}
	return &http.Client{Timeout: 20 * time.Second}
}

type zarinpalRequest struct {
	MerchantID  string `json:"merchant_id"`
	Amount      int64  `json:"amount"`
	CallbackURL string `json:"callback_url,omitempty"`
	Description string `json:"description,omitempty"`
	Metadata    struct {
		Mobile string `json:"mobile,omitempty"`
	} `json:"metadata,omitempty"`
}

// zarinpalPayload is the success shape.
type zarinpalPayload struct {
	Code      int    `json:"code"`
	Message   string `json:"message"`
	Authority string `json:"authority"`
	RefID     int64  `json:"ref_id"`
}

// zarinpalResponse keeps data raw: on success Zarinpal sends an object, on failure it sends an
// empty ARRAY ("data":[]). Decoding straight into a struct made every rejection look like a
// transport error, so a declined payment could not be told apart from an unreachable gateway.
type zarinpalResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors json.RawMessage `json:"errors"`
}

// payload decodes data when it is an object; an array or null yields the zero payload.
func (r zarinpalResponse) payload() zarinpalPayload {
	var p zarinpalPayload
	if len(r.Data) > 0 && r.Data[0] == '{' {
		_ = json.Unmarshal(r.Data, &p)
	}
	return p
}

func (z ZarinpalGateway) Start(ctx context.Context, req StartPayment) (StartResult, error) {
	body := zarinpalRequest{
		MerchantID:  z.MerchantID,
		Amount:      req.Amount,
		CallbackURL: z.CallbackURL,
		Description: req.Description,
	}
	body.Metadata.Mobile = req.Mobile

	var out zarinpalResponse
	if err := z.post(ctx, z.BaseURL+"/request.json", body, &out); err != nil {
		return StartResult{}, err
	}
	p := out.payload()
	if p.Code != 100 {
		return StartResult{}, fmt.Errorf("%w: code %d %s", ErrGatewayRejected, p.Code, p.Message)
	}
	return StartResult{
		Authority:   p.Authority,
		RedirectURL: z.StartURL + p.Authority,
	}, nil
}

func (z ZarinpalGateway) Verify(ctx context.Context, req VerifyPayment) (VerifyResult, error) {
	body := zarinpalRequest{MerchantID: z.MerchantID, Amount: req.Amount}
	// The verify endpoint takes the authority in the path.
	var out zarinpalResponse
	if err := z.post(ctx, z.BaseURL+"/verify.json?authority="+req.Authority, body, &out); err != nil {
		return VerifyResult{}, err
	}
	// 100 = verified now, 101 = already verified. Both mean the money arrived; treating 101 as a
	// failure would double-charge a customer who refreshed the callback page.
	p := out.payload()
	if p.Code != 100 && p.Code != 101 {
		return VerifyResult{Paid: false}, nil
	}
	return VerifyResult{
		Paid:      true,
		Reference: fmt.Sprintf("%d", p.RefID),
		Amount:    req.Amount,
	}, nil
}

func (z ZarinpalGateway) post(ctx context.Context, url string, in, out any) error {
	buf, err := json.Marshal(in)
	if err != nil {
		return fmt.Errorf("commerce: gateway encode: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return fmt.Errorf("commerce: gateway request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := z.client().Do(httpReq)
	if err != nil {
		return fmt.Errorf("commerce: gateway unreachable: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("commerce: gateway read: %w", err)
	}
	if resp.StatusCode >= 500 {
		return fmt.Errorf("commerce: gateway %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}
	// Zarinpal answers 4xx with a JSON error body; decode it so the code is visible.
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("commerce: gateway decode (status %d): %w", resp.StatusCode, err)
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
