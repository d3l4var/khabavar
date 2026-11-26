// Package config loads environment variables into a typed Config.
// Slot names live in .env.example; values are NEVER committed (DEP §۵).
package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Role               string // identity | api | worker (ADR-DB-002)
	DBURL              string
	DBMaxOpenConns     int
	OTPTTLSeconds      int
	OTPMaxPerHour      int
	TokenSigningKey    string
	AccessTokenTTL     int // seconds
	RefreshTokenTTL    int // seconds
	HTTPAddr           string
	LogLevel           string
	AuthCodeTTLSeconds int    // OIDC authorization-code TTL (seconds, default 600)
	OIDCIssuer         string // id_token.iss (e.g. https://id.khabavar.ir)
	WorkerInterval     int    // outbox poll interval, seconds (default 5)
	WorkerBatchSize    int    // outbox claim batch size (default 100)
	PaymentAdapter     string
	SmsAdapter         string
	StorageAdapter     string
	StorageLocalRoot   string
	Cache              string
}

func Load() (Config, error) {
	role := getenv("KH_ROLE", "api")
	if role != "identity" && role != "api" && role != "worker" {
		return Config{}, fmt.Errorf("KH_ROLE must be identity|api|worker, got %q", role)
	}
	c := Config{
		Role:               role,
		DBURL:              os.Getenv("KH_DB_URL"),
		DBMaxOpenConns:     getenvInt("KH_DB_MAX_OPEN_CONNS", 25),
		OTPTTLSeconds:      getenvInt("KH_OTP_TTL_SECONDS", 120),
		OTPMaxPerHour:      getenvInt("KH_OTP_MAX_PER_HOUR", 5),
		TokenSigningKey:    os.Getenv("KH_TOKEN_SIGNING_KEY"),
		AccessTokenTTL:     getenvInt("KH_ACCESS_TOKEN_TTL_SECONDS", 900),
		RefreshTokenTTL:    getenvInt("KH_REFRESH_TOKEN_TTL_SECONDS", 2592000),
		HTTPAddr:           getenv("KH_HTTP_ADDR", ":8080"),
		LogLevel:           getenv("KH_LOG_LEVEL", "info"),
		AuthCodeTTLSeconds: getenvInt("KH_AUTH_CODE_TTL_SECONDS", 600),
		OIDCIssuer:         getenv("KH_OIDC_ISSUER", "https://id.khabavar.ir"),
		WorkerInterval:     getenvInt("KH_WORKER_INTERVAL_SECONDS", 5),
		WorkerBatchSize:    getenvInt("KH_WORKER_BATCH_SIZE", 100),
		PaymentAdapter:     getenv("KH_PAYMENT_ADAPTER", "mock"),
		SmsAdapter:         getenv("KH_SMS_ADAPTER", "mock"),
		StorageAdapter:     getenv("KH_STORAGE_ADAPTER", "local"),
		StorageLocalRoot:   getenv("KH_STORAGE_LOCAL_ROOT", "/var/lib/khabavar/media"),
		Cache:              getenv("KH_CACHE", "none"),
	}
	if c.DBURL == "" {
		return c, fmt.Errorf("KH_DB_URL is required")
	}
	return c, nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func getenvInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
