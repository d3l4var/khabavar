-- 000015_idempotency.up.sql
-- ADR-ARCH-008: a retried request with the same Idempotency-Key must return the first
-- result and must not repeat the effect. Money-touching routes (checkout, payments)
-- need this at the request level, not only on payment_attempts.

BEGIN;

CREATE TABLE IF NOT EXISTS idempotency_keys (
    key           VARCHAR(100) PRIMARY KEY,
    scope         VARCHAR(80)  NOT NULL,          -- e.g. POST /api/v1/checkout
    request_hash  TEXT         NOT NULL,          -- sha256 of the canonical body
    response_body JSONB,                          -- NULL while the request is in flight
    status_code   INTEGER,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    expires_at    TIMESTAMPTZ  NOT NULL           -- contract: keep at least 24h
);

CREATE INDEX IF NOT EXISTS ix_idempotency_expiry ON idempotency_keys (expires_at);

COMMIT;
