package config

import (
	"strings"
	"testing"
)

// TestValidate_RefusesUnusableConfig pins the rule that a service must not start healthy and
// then fail on the first request. Every case here used to start and break later.
func TestValidate_RefusesUnusableConfig(t *testing.T) {
	good := Config{
		Role:            "api",
		DBURL:           "postgres://x",
		TokenSigningKey: strings.Repeat("k", 32),
		AccessTokenTTL:  900,
		PaymentAdapter:  "mock",
		StorageAdapter:  "local",
	}
	if err := good.validate(); err != nil {
		t.Fatalf("a valid config was rejected: %v", err)
	}

	cases := []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{"no dsn", func(c *Config) { c.DBURL = "" }, "KH_DB_URL"},
		{"short signing key", func(c *Config) { c.TokenSigningKey = "short" }, "KH_TOKEN_SIGNING_KEY"},
		{"empty signing key", func(c *Config) { c.TokenSigningKey = "" }, "KH_TOKEN_SIGNING_KEY"},
		{"zarinpal without merchant", func(c *Config) { c.PaymentAdapter = "zarinpal" }, "KH_ZARINPAL_MERCHANT_ID"},
		{"s3 without bucket", func(c *Config) { c.StorageAdapter = "s3" }, "KH_S3_BUCKET"},
		{"zero token ttl", func(c *Config) { c.AccessTokenTTL = 0 }, "KH_ACCESS_TOKEN_TTL_SECONDS"},
	}
	for _, tc := range cases {
		cfg := good
		tc.mutate(&cfg)
		err := cfg.validate()
		if err == nil {
			t.Fatalf("%s: accepted an unusable config", tc.name)
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%s: error %q does not name %s", tc.name, err, tc.want)
		}
	}
}

// TestLoad_RejectsUnknownRole: a typo in KH_ROLE must not silently run the api role.
func TestLoad_RejectsUnknownRole(t *testing.T) {
	t.Setenv("KH_ROLE", "apis")
	if _, err := Load(); err == nil {
		t.Fatal("accepted KH_ROLE=apis")
	}
}
