// Package main is the single entry point for all three units (ADR-DB-002).
// Each unit (khabavar-identity / khabavar-api / khabavar-worker) runs the same
// binary with KH_ROLE=identity|api|worker; the unit split is for independent
// restart and resource isolation, not code separation (DEP v1.1 §۳).
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/khabavar/khabavar/internal/modules/audit"
	"github.com/khabavar/khabavar/internal/modules/authorization"
	"github.com/khabavar/khabavar/internal/modules/catalog"
	"github.com/khabavar/khabavar/internal/modules/commerce"
	"github.com/khabavar/khabavar/internal/modules/content"
	"github.com/khabavar/khabavar/internal/modules/ecosystem"
	"github.com/khabavar/khabavar/internal/modules/fulfillment"
	"github.com/khabavar/khabavar/internal/modules/identity"
	"github.com/khabavar/khabavar/internal/modules/inventory"
	"github.com/khabavar/khabavar/internal/modules/media"
	"github.com/khabavar/khabavar/internal/modules/notifications"
	"github.com/khabavar/khabavar/internal/modules/pricing"
	"github.com/khabavar/khabavar/internal/modules/suppliers"
	"github.com/khabavar/khabavar/internal/modules/worker"
	"github.com/khabavar/khabavar/internal/platform/config"
	"github.com/khabavar/khabavar/internal/platform/db"
	"github.com/khabavar/khabavar/internal/platform/health"
	khlog "github.com/khabavar/khabavar/internal/platform/log"
	"github.com/khabavar/khabavar/internal/platform/ratelimit"
	"github.com/khabavar/khabavar/internal/platform/security"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}
	logger := khlog.New(cfg.LogLevel)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := db.Open(ctx, cfg.DBURL, cfg.DBMaxOpenConns)
	if err != nil {
		logger.Error("open db", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	switch cfg.Role {
	case "identity":
		runIdentity(ctx, cfg, logger, pool)
	case "api":
		runAPI(ctx, cfg, logger, pool)
	case "worker":
		runWorker(ctx, cfg, logger, pool)
	}
}

// runIdentity mounts the OTP endpoints and the OIDC Authorization Code + PKCE flow
// (F1 — OTP login, SAD §۷; CIAM §۳-۵, R-07 §oauth). It serves the identity unit
// (KH_ROLE=identity) and is the only unit that exposes /oauth/*.
func runIdentity(ctx context.Context, cfg config.Config, logger *slog.Logger, pool *db.Pool) {
	if cfg.TokenSigningKey == "" {
		logger.Error("KH_TOKEN_SIGNING_KEY is required (HMAC for OTP/OIDC secrets)")
		os.Exit(1)
	}
	store := identity.NewDBStore(pool.Pool)
	rl := ratelimit.New(cfg.OTPMaxPerHour, time.Hour)
	svc := identity.NewService(store, cfg.TokenSigningKey,
		time.Duration(cfg.OTPTTLSeconds)*time.Second,
		time.Duration(cfg.AccessTokenTTL)*time.Second,
		5, rl)

	oidcStore := identity.NewDBOIDCStore(pool.Pool)
	resolver := identity.DBSubjectResolver(pool.Pool, cfg.TokenSigningKey)
	oidcSvc := identity.NewOIDCService(oidcStore, resolver, cfg.TokenSigningKey, cfg.OIDCIssuer,
		time.Duration(cfg.AuthCodeTTLSeconds)*time.Second,
		time.Duration(cfg.AccessTokenTTL)*time.Second,
		time.Duration(cfg.RefreshTokenTTL)*time.Second)

	mux := http.NewServeMux()
	hh := health.New(pool)
	hh.Mount(mux)
	mux.HandleFunc("POST /api/v1/auth/otp/start", identity.OTPStartHandler(svc, mockSmsDeliver(logger)))
	mux.HandleFunc("POST /api/v1/auth/otp/verify", identity.OTPVerifyHandler(svc))
	// Staff login for the panel (email + password → opaque session token).
	mux.HandleFunc("POST /api/v1/auth/admin/login", identity.AdminLoginHandler(svc, identity.NewDBAdminLoginStore(pool.Pool)))
	mux.HandleFunc("GET /oauth/authorize", identity.AuthorizeHandler(oidcSvc))
	mux.HandleFunc("POST /oauth/token", identity.TokenHandler(oidcSvc))
	hh.SetReady(true)
	serve(ctx, cfg.HTTPAddr, mux, logger)
}

// runAPI mounts the storefront read endpoints and the staff (admin) endpoints.
// Storefront reads are public; every /api/v1/admin/* route goes through apiMux.admin,
// which authenticates the Bearer token and checks the seeded permission code for that
// route (IAM §۱). A denial is written to the audit log (IAM §۷).
func runAPI(ctx context.Context, cfg config.Config, logger *slog.Logger, pool *db.Pool) {
	if cfg.TokenSigningKey == "" {
		logger.Error("KH_TOKEN_SIGNING_KEY is required (Bearer token verification)")
		os.Exit(1)
	}
	mux := http.NewServeMux()
	hh := health.New(pool)
	hh.Mount(mux)

	// RBAC wiring: token → admin identity → seeded permission matrix (IAM §۱/§۴).
	resolver := identity.DBSubjectResolver(pool.Pool, cfg.TokenSigningKey)
	resolve := func(ctx context.Context, bearer string) (string, string, bool) {
		s, ok := resolver(ctx, bearer)
		return s.Kind, s.ID, ok
	}
	checker := authorization.NewDBChecker(pool.Pool)
	auditor := audit.NewRecorder(pool.Pool)
	api := newAPIMux(mux, newAdminGuard(resolve, checker, auditor))

	catSvc := catalog.NewService(catalog.NewDBStore(pool.Pool))
	catSvc.SetSearchStore(catalog.NewDBStore(pool.Pool))
	catSvc.SetUpdateStore(catalog.NewDBStore(pool.Pool))
	catSvc.SetAuditor(auditor)
	contentSvc := content.NewService(content.NewDBStore(pool.Pool))
	contentSvc.SetSearchStore(content.NewDBStore(pool.Pool))

	comSvc := commerce.NewService(commerce.NewDBStore(pool.Pool), commerce.NewDBEventEnqueuer(pool.Pool))
	comSvc.SetRefundStore(commerce.NewDBStore(pool.Pool))
	comSvc.SetAttemptStore(commerce.NewDBStore(pool.Pool))
	comSvc.SetPaymentProvider(cfg.PaymentAdapter)
	comSvc.SetAuditor(auditor)

	mediaSvc := media.NewService(media.NewDBStore(pool.Pool), media.NewLocalStorage(cfg.StorageLocalRoot))

	pricingSvc := pricing.NewService(pricing.NewDBStore(pool.Pool))
	pricingSvc.SetAuditor(auditor)

	svcs := services{
		catalog:     catSvc,
		pricing:     pricingSvc,
		commerce:    comSvc,
		suppliers:   suppliers.NewService(suppliers.NewDBStore(pool.Pool)),
		inventory:   inventory.NewService(inventory.NewDBStore(pool.Pool)),
		fulfillment: fulfillment.NewService(fulfillment.NewDBStore(pool.Pool)),
		media:       mediaSvc,
		content:     contentSvc,
		ecosystem:   ecosystem.NewService(catalogSearch{catSvc}, articleSearch{contentSvc}),
		catList:     catalog.NewAdminListStore(pool.Pool),
		invAdmin:    inventory.NewAdminStore(pool.Pool),
		iamAdmin:    identity.NewAdminStore(pool.Pool),
		auditList:   audit.NewListStore(pool.Pool),
		orderList:   commerce.NewOrderListStore(pool.Pool),
	}
	mountRoutes(api, svcs)

	hh.SetReady(true)
	serve(ctx, cfg.HTTPAddr, mux, logger)
}

// runWorker polls the domain_events outbox (ADR-ARCH-007), dispatches each event to
// the handler registered for its event_type, and applies the retry/dead-letter
// policy. It also serves a health endpoint. Reservation-expiry and cleanup jobs are
// added in P5+ (DEP §۳, GUIDE §۴.۴).
func runWorker(ctx context.Context, cfg config.Config, logger *slog.Logger, pool *db.Pool) {
	mux := http.NewServeMux()
	hh := health.New(pool)
	hh.Mount(mux)
	hh.SetReady(true)
	go serve(ctx, cfg.HTTPAddr, mux, logger)
	runOutboxDispatcher(ctx, cfg, logger, pool)
}

// runOutboxDispatcher drains domain_events and dispatches to per-event-type handlers.
// Customer-facing events go through the notifications module (ADR-ARCH-011): the mock
// SMS adapter is wired until a real provider is chosen, so the delivery path is the
// production one and only the adapter is swapped by configuration.
func runOutboxDispatcher(ctx context.Context, cfg config.Config, logger *slog.Logger, pool *db.Pool) {
	store := worker.NewDBOutbox(pool.Pool)
	d := worker.NewDispatcher(store, logger, cfg.WorkerBatchSize, 5)

	sms := &notifications.MockSmsAdapter{Logger: logger}
	email := &notifications.MockEmailAdapter{Logger: logger}
	notifier := notifications.NewDispatcher(sms, email)
	handler := notifyHandler(notifier, logger)

	// Known commerce/fulfillment event types (R-06 §۱۰); fallback covers anything else.
	d.Register("OrderCreated", handler)
	d.Register("OrderPaid", handler)
	d.Register("OrderCancelled", handler)
	d.Register("OrderConfirmed", handler)
	d.Register("PaymentCaptured", handler)
	d.Register("ShipmentDispatched", handler)
	d.Register("OrderShipped", handler)
	d.Register("RefundIssued", handler)
	d.SetFallback(handler)
	d.Run(ctx, time.Duration(cfg.WorkerInterval)*time.Second)
}

// notifyHandler adapts the notifications dispatcher to the worker's Handler shape.
// A delivery failure returns the error so the outbox retries (then dead-letters).
func notifyHandler(n *notifications.Dispatcher, logger *slog.Logger) worker.Handler {
	return func(ctx context.Context, e worker.Event) error {
		var payload map[string]any
		if len(e.Payload) > 0 {
			if err := json.Unmarshal(e.Payload, &payload); err != nil {
				// A malformed payload must not wedge the queue: log and acknowledge.
				logger.Warn("worker: unparseable event payload", "event_type", e.EventType, "err", err)
				return nil
			}
		}
		return n.Dispatch(ctx, notifications.Event{
			EventType:   e.EventType,
			AggregateID: e.AggregateID,
			Payload:     payload,
		})
	}
}

func serve(ctx context.Context, addr string, mux *http.ServeMux, logger *slog.Logger) {
	// Wrap the mux with security headers + CORS (R-19 P9). SecurityHeaders adds
	// HSTS, X-Content-Type-Options, CSP, Cache-Control. CORS allows the four
	// khabavar subdomains to make authenticated cross-origin requests.
	handler := security.Chain(mux,
		security.CORS(security.DefaultCORSConfig()),
		security.SecurityHeaders,
	)
	srv := &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		logger.Info("http listen", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("http server", "err", err)
			os.Exit(1)
		}
	}()
	<-ctx.Done()
	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutCtx)
}

// mockSmsDeliver returns a deliver func for the Mock SmsAdapter (ADR-ARCH-011).
// v1 logs the code length at debug level; the real adapter sends via the SMS provider.
// The raw code is never logged in production (CIAM §۳-۴) — only its length here.
func mockSmsDeliver(logger *slog.Logger) func(phone, code string) {
	return func(phone, code string) {
		logger.Debug("sms mock deliver", "phone", phone, "code_len", len(code))
	}
}
