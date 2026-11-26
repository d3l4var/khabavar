-- 000001_identity.up.sql
-- ماژول Identity — مطابق DB v2 §۵ (customers, customer_addresses) + §۲ (roles, permissions)
-- + نهایی‌سازی IAM (KH-IAM-001): OTP، نشست سرویس‌محور، OIDC clients، کاربران پرسنل

BEGIN;

-- مشتریان (DB v2 §5.1)
CREATE TABLE IF NOT EXISTS customers (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    phone              VARCHAR(20),            -- یکتا در صورت وجود؛ قالب استاندارد داخلی
    email              VARCHAR(255),           -- اختیاری
    full_name          VARCHAR(200),
    status             TEXT NOT NULL DEFAULT 'ACTIVE'
                       CHECK (status IN ('ACTIVE','BLOCKED','DELETED')),
    phone_verified_at  TIMESTAMPTZ,
    marketing_consent  BOOLEAN NOT NULL DEFAULT FALSE,  -- CIAM §۹ رضایت جداگانه بازاریابی
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- §۱۵: phone UNIQUE — جزئی، چون اختیاری است
CREATE UNIQUE INDEX IF NOT EXISTS uq_customers_phone ON customers (phone) WHERE phone IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_customers_email ON customers (email) WHERE email IS NOT NULL;

-- آدرس‌های مشتری (DB v2 §5.2) — هنگام سفارش Snapshot می‌شود
CREATE TABLE IF NOT EXISTS customer_addresses (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id      UUID NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    province         VARCHAR(100) NOT NULL,
    city             VARCHAR(100) NOT NULL,
    postal_code      VARCHAR(20),
    address_line     TEXT NOT NULL,
    plaque           VARCHAR(20),
    unit             VARCHAR(20),
    recipient_name   VARCHAR(200),
    recipient_phone  VARCHAR(20),
    is_default       BOOLEAN NOT NULL DEFAULT FALSE,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_addresses_customer ON customer_addresses (customer_id);

-- نقش‌ها و مجوزها (DB v2 §۲ tree: roles, permissions)
CREATE TABLE IF NOT EXISTS roles (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code        VARCHAR(50) NOT NULL UNIQUE,
    name        VARCHAR(100) NOT NULL,
    description TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS permissions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code        VARCHAR(100) NOT NULL UNIQUE,   -- مثل catalog.product.publish
    resource    VARCHAR(50) NOT NULL,
    action      VARCHAR(50) NOT NULL,
    description TEXT
);
CREATE TABLE IF NOT EXISTS role_permissions (
    role_id       UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission_id UUID NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    PRIMARY KEY (role_id, permission_id)
);

-- کاربران پرسنل (پنل مدیریت — CIAM §۶-۷؛ مکمل جدول customers برای مشتریان)
CREATE TABLE IF NOT EXISTS admin_users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email         VARCHAR(255) NOT NULL UNIQUE,
    full_name     VARCHAR(200) NOT NULL,
    password_hash TEXT,                            -- نسخه اول: ورود پنل با OTP؛ فیلد برای آینده
    status        TEXT NOT NULL DEFAULT 'ACTIVE'
                  CHECK (status IN ('ACTIVE','SUSPENDED','DEACTIVATED')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS admin_user_roles (
    admin_user_id UUID NOT NULL REFERENCES admin_users(id) ON DELETE CASCADE,
    role_id       UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    PRIMARY KEY (admin_user_id, role_id)
);

-- کلاینت‌های OIDC (CIAM §۳ — Redirect URI whitelist، یک کلاینت به ازای هر سرویس)
CREATE TABLE IF NOT EXISTS oauth_clients (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    client_id     VARCHAR(64) NOT NULL UNIQUE,
    service_code  VARCHAR(50) NOT NULL UNIQUE,     -- main | shop | magazine | admin
    name          VARCHAR(200) NOT NULL,
    redirect_uris TEXT[] NOT NULL,                 -- whitelist دقیق
    scopes        TEXT[] NOT NULL DEFAULT '{}',
    is_active     BOOLEAN NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- چالش‌های OTP (CIAM §۳-۴ — کد خام هرگز ذخیره نمی‌شود؛ فقط هش)
CREATE TABLE IF NOT EXISTS otp_challenges (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    phone         VARCHAR(20) NOT NULL,
    purpose       TEXT NOT NULL DEFAULT 'LOGIN'
                  CHECK (purpose IN ('LOGIN','VERIFY_PHONE','ADMIN_LOGIN')),
    code_hash     TEXT NOT NULL,
    attempts      INT NOT NULL DEFAULT 0,
    max_attempts  INT NOT NULL DEFAULT 5,
    expires_at    TIMESTAMPTZ NOT NULL,
    consumed_at   TIMESTAMPTZ,
    ip_address    INET,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_otp_phone_created ON otp_challenges (phone, created_at DESC);

-- نشست‌ها (CIAM §۵ — نشست مستقل به ازای هر سرویس؛ ابطال سمت سرور)
CREATE TABLE IF NOT EXISTS identity_sessions (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id    UUID REFERENCES customers(id) ON DELETE CASCADE,
    admin_user_id  UUID REFERENCES admin_users(id) ON DELETE CASCADE,
    service_code   VARCHAR(50) NOT NULL,
    token_hash     TEXT NOT NULL UNIQUE,
    authorization_code_hash TEXT,                  -- کد یک‌بارمصرف کوتاه‌عمر جریان OIDC
    ip_address     INET,
    user_agent     TEXT,
    expires_at     TIMESTAMPTZ NOT NULL,
    revoked_at     TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (customer_id IS NOT NULL OR admin_user_id IS NOT NULL),
    CHECK (customer_id IS NULL OR admin_user_id IS NULL)
);
CREATE INDEX IF NOT EXISTS ix_sessions_customer ON identity_sessions (customer_id, service_code);
CREATE INDEX IF NOT EXISTS ix_sessions_admin ON identity_sessions (admin_user_id, service_code);

-- کد یک‌بارمصرف Authorization Code جریان OIDC (Auth Code + PKCE)
CREATE TABLE IF NOT EXISTS oauth_authorization_codes (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    client_id          VARCHAR(64) NOT NULL REFERENCES oauth_clients(client_id),
    code_hash          TEXT NOT NULL UNIQUE,
    customer_id        UUID REFERENCES customers(id) ON DELETE CASCADE,
    admin_user_id      UUID REFERENCES admin_users(id) ON DELETE CASCADE,
    redirect_uri       TEXT NOT NULL,
    code_challenge     TEXT,                       -- PKCE S256
    code_challenge_method VARCHAR(10) DEFAULT 'S256',
    scopes             TEXT[] NOT NULL DEFAULT '{}',
    expires_at         TIMESTAMPTZ NOT NULL,
    consumed_at        TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (customer_id IS NOT NULL OR admin_user_id IS NOT NULL)
);

-- updated_at trigger عمومی برای جدول‌های این ماژول
CREATE OR REPLACE FUNCTION khabavar_set_updated_at() RETURNS trigger AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_customers_updated      BEFORE UPDATE ON customers FOR EACH ROW EXECUTE FUNCTION khabavar_set_updated_at();
CREATE TRIGGER trg_addresses_updated      BEFORE UPDATE ON customer_addresses FOR EACH ROW EXECUTE FUNCTION khabavar_set_updated_at();
CREATE TRIGGER trg_admin_users_updated    BEFORE UPDATE ON admin_users FOR EACH ROW EXECUTE FUNCTION khabavar_set_updated_at();
CREATE TRIGGER trg_oauth_clients_updated  BEFORE UPDATE ON oauth_clients FOR EACH ROW EXECUTE FUNCTION khabavar_set_updated_at();

COMMIT;
