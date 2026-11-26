// Package main is the single entry point for all three units (ADR-DB-002).
// Each unit (khabavar-identity / khabavar-api / khabavar-worker) runs the same
// binary with KH_ROLE=identity|api|worker; the unit split is for independent
// restart and resource isolation, not code separation (DEP v1.1 §۳).
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/khabavar/khabavar/internal/modules/catalog"
	"github.com/khabavar/khabavar/internal/modules/commerce"
	"github.com/khabavar/khabavar/internal/modules/identity"
	"github.com/khabavar/khabavar/internal/modules/pricing"
	"github.com/khabavar/khabavar/internal/modules/worker"
	"github.com/khabavar/khabavar/internal/platform/config"
	"github.com/khabavar/khabavar/internal/platform/db"
	"github.com/khabavar/khabavar/internal/platform/health"
	khlog "github.com/khabavar/khabavar/internal/platform/log"
	"github.com/khabavar/khabavar/internal/platform/ratelimit"
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
	mux.HandleFunc("GET /oauth/authorize", identity.AuthorizeHandler(oidcSvc))
	mux.HandleFunc("POST /oauth/token", identity.TokenHandler(oidcSvc))
	hh.SetReady(true)
	serve(ctx, cfg.HTTPAddr, mux, logger)
}

// runAPI mounts the storefront read endpoints. Catalog (P5a) and pricing storefront
// reads (P5b) are wired now; commerce endpoints (P4) are wired in P6. Admin endpoints
// (catalog writes, pricing economics) land behind authorization.Require in the P5b-admin pass.
func runAPI(ctx context.Context, cfg config.Config, logger *slog.Logger, pool *db.Pool) {
	mux := http.NewServeMux()
	hh := health.New(pool)
	hh.Mount(mux)

	catSvc := catalog.NewService(catalog.NewDBStore(pool.Pool))
	mux.HandleFunc("GET /api/v1/categories", catalog.CategoryTreeHandler(catSvc))
	mux.HandleFunc("GET /api/v1/categories/{id}/products", catalog.ListProductsByCategoryHandler(catSvc))
	mux.HandleFunc("GET /api/v1/products/{id}", catalog.ProductDetailHandler(catSvc))

	priceSvc := pricing.NewService(pricing.NewDBStore(pool.Pool))
	mux.HandleFunc("GET /api/v1/variants/{id}/price", pricing.StorefrontPriceHandler(priceSvc))

	// Commerce (P4): order lifecycle + payment + outbox events (R-06 §۱۰)
	comSvc := commerce.NewService(commerce.NewDBStore(pool.Pool), commerce.NewDBEventEnqueuer(pool.Pool))
	mux.Handle("POST /api/v1/orders", commerce.CreateOrderHandler(comSvc))
	mux.Handle("GET /api/v1/orders/{id}", commerce.GetOrderHandler(comSvc))
	mux.Handle("GET /api/v1/orders/by-number/{number}", commerce.GetOrderByNumberHandler(comSvc))
	mux.Handle("POST /api/v1/orders/{id}/payment-intents", commerce.CreatePaymentIntentHandler(comSvc))
	mux.Handle("POST /api/v1/orders/{id}/confirm-payment", commerce.ConfirmPaymentHandler(comSvc))
	mux.Handle("POST /api/v1/orders/{id}/cancel", commerce.CancelOrderHandler(comSvc))

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
// v1 ships a mock notifications handler that logs the event; P5+ wires the real
// notifications module (SmsAdapter/EmailAdapter) here.
func runOutboxDispatcher(ctx context.Context, cfg config.Config, logger *slog.Logger, pool *db.Pool) {
	store := worker.NewDBOutbox(pool.Pool)
	d := worker.NewDispatcher(store, logger, cfg.WorkerBatchSize, 5)
	handler := mockDispatch(logger)
	// Known commerce/fulfillment event types (R-06 §۱۰); fallback covers anything else.
	d.Register("OrderCreated", handler)
	d.Register("OrderPaid", handler)
	d.Register("OrderCancelled", handler)
	d.Register("OrderConfirmed", handler)
	d.Register("PaymentCaptured", handler)
	d.Register("ShipmentDispatched", handler)
	d.SetFallback(handler)
	d.Run(ctx, time.Duration(cfg.WorkerInterval)*time.Second)
}

// mockDispatch is the v1 notifications handler (ADR-ARCH-011). It logs the event type
// and aggregate id; the real adapter sends the SMS/email in P5.
func mockDispatch(logger *slog.Logger) worker.Handler {
	return func(_ context.Context, e worker.Event) error {
		logger.Info("worker: mock dispatch", "event_type", e.EventType, "aggregate_id", e.AggregateID)
		return nil
	}
}

func serve(ctx context.Context, addr string, mux *http.ServeMux, logger *slog.Logger) {
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
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
