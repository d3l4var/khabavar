-- 000011_oidc_refresh_tokens.up.sql
-- ماژول Identity (مکمل 000001) — Refresh Tokenهای چرخشی OIDC (CIAM §۵)
-- توکن refresh خام هرگز ذخیره نمی‌شود؛ فقط token_hash. هر استفاده → مصرف + صدور جدید.

BEGIN;

CREATE TABLE IF NOT EXISTS oauth_refresh_tokens (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    token_hash     TEXT NOT NULL UNIQUE,
    client_id      VARCHAR(64) NOT NULL REFERENCES oauth_clients(client_id),
    session_id     UUID REFERENCES identity_sessions(id) ON DELETE CASCADE,
    customer_id    UUID REFERENCES customers(id) ON DELETE CASCADE,
    admin_user_id  UUID REFERENCES admin_users(id) ON DELETE CASCADE,
    scopes         TEXT[] NOT NULL DEFAULT '{}',
    expires_at     TIMESTAMPTZ NOT NULL,
    consumed_at    TIMESTAMPTZ,                       -- چرخش: پس از تبادل،标记 شده
    revoked_at     TIMESTAMPTZ,                       -- ابطال صریح (logout/compromise)
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (customer_id IS NOT NULL OR admin_user_id IS NOT NULL),
    CHECK (customer_id IS NULL OR admin_user_id IS NULL)
);

-- اسکن سریع توکن‌های منتشرنشده/منقضی‌نشده برای اعتبارسنجی در grant_type=refresh_token
CREATE INDEX IF NOT EXISTS ix_refresh_tokens_hash
    ON oauth_refresh_tokens (token_hash) WHERE consumed_at IS NULL AND revoked_at IS NULL;
CREATE INDEX IF NOT EXISTS ix_refresh_tokens_session
    ON oauth_refresh_tokens (session_id);

COMMIT;
