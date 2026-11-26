=== 000001_identity.up.sql ===
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

=== 000002_catalog.up.sql ===
-- 000002_catalog.up.sql
-- ماژول Catalog — مطابق DB v2 §۶ + §۲ (brands, media) + قواعد مدل A از PRD §۲ (MA-1، MA-6)

BEGIN;

-- دارایی‌های رسانه‌ای عمومی (برای تصویر دسته‌بندی و برند؛ کلید فایل در Object Storage)
CREATE TABLE IF NOT EXISTS media_assets (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    media_type  TEXT NOT NULL DEFAULT 'IMAGE' CHECK (media_type IN ('IMAGE','VIDEO','DOCUMENT')),
    storage_key TEXT NOT NULL,
    alt_text    TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- برندها (DB v2 §۲ tree: brands)
CREATE TABLE IF NOT EXISTS brands (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       VARCHAR(200) NOT NULL,
    slug       VARCHAR(255) NOT NULL UNIQUE,
    logo_id    UUID REFERENCES media_assets(id),
    status     TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','INACTIVE')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- دسته‌بندی‌ها (DB v2 §6.1 — درختی با parent_id)
CREATE TABLE IF NOT EXISTS categories (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    parent_id   UUID REFERENCES categories(id),
    name        VARCHAR(200) NOT NULL,
    slug        VARCHAR(255) NOT NULL UNIQUE,
    description TEXT,
    image_id    UUID REFERENCES media_assets(id),
    sort_order  INT NOT NULL DEFAULT 0,
    status      TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','INACTIVE')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_categories_parent ON categories (parent_id);   -- §۱۵

-- محصولات (DB v2 §6.2 — وضعیت‌ها عیناً از سند)
CREATE TABLE IF NOT EXISTS products (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title                VARCHAR(255) NOT NULL,
    slug                 VARCHAR(255) NOT NULL UNIQUE,                       -- §۱۵
    brand_id             UUID REFERENCES brands(id),
    short_description    TEXT,
    description          TEXT,
    manufacturer         VARCHAR(255),
    country_of_origin    VARCHAR(100),
    -- قواعد مدل A (PRD §۲ — MA-1، MA-6): بدون توجیه خواب/منبع مشخصات، انتشار ممنوع
    sleep_relevance      TEXT,          -- توجیه ارتباط واقعی محصول با خواب (MA-1)
    spec_source          TEXT,          -- منبع مشخصات فنی (MA-6)
    spec_last_reviewed_at TIMESTAMPTZ,  -- تاریخ آخرین بازبینی مشخصات (MA-6)
    status               TEXT NOT NULL DEFAULT 'DRAFT'
                         CHECK (status IN ('DRAFT','DATA_REVIEW','APPROVED','PUBLISHED','PAUSED','DISCONTINUED')),
    published_at         TIMESTAMPTZ,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_products_publish_gates CHECK (
        status <> 'PUBLISHED'
        OR (sleep_relevance IS NOT NULL AND sleep_relevance <> ''
            AND spec_source IS NOT NULL AND spec_source <> ''
            AND spec_last_reviewed_at IS NOT NULL)
    )
);

-- رابطه محصول-دسته‌بندی (چندبه‌چند — ERD: PRODUCTS classified CATEGORIES)
CREATE TABLE IF NOT EXISTS product_categories (
    product_id  UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    category_id UUID NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
    PRIMARY KEY (product_id, category_id)
);
CREATE INDEX IF NOT EXISTS ix_product_categories_cat ON product_categories (category_id);

-- Variantهای قابل فروش (DB v2 §6.3)
CREATE TABLE IF NOT EXISTS product_variants (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id   UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    sku          VARCHAR(100) NOT NULL UNIQUE,                                 -- §۱۵
    barcode      VARCHAR(100),
    attributes   JSONB,                       -- فقط مشخصات انعطاف‌پذیر غیرهسته‌ای (§6.3)
    weight_grams INT CHECK (weight_grams IS NULL OR weight_grams >= 0),
    length_mm    INT CHECK (length_mm  IS NULL OR length_mm  >= 0),
    width_mm     INT CHECK (width_mm   IS NULL OR width_mm   >= 0),
    height_mm    INT CHECK (height_mm  IS NULL OR height_mm  >= 0),
    status       TEXT NOT NULL DEFAULT 'DRAFT'
                 CHECK (status IN ('DRAFT','ACTIVE','INACTIVE','DISCONTINUED')),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_variants_product ON product_variants (product_id);

-- ویژگی‌های ساختاریافته (DB v2 §6.4)
CREATE TABLE IF NOT EXISTS attribute_definitions (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code          VARCHAR(100) NOT NULL UNIQUE,
    name          VARCHAR(200) NOT NULL,
    data_type     TEXT NOT NULL CHECK (data_type IN ('TEXT','NUMBER','BOOLEAN','OPTION')),
    unit          VARCHAR(30),                 -- برای مقادیر عددی واحد الزامی است (§6.4)
    is_filterable BOOLEAN NOT NULL DEFAULT FALSE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS attribute_options (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    attribute_id UUID NOT NULL REFERENCES attribute_definitions(id) ON DELETE CASCADE,
    value        VARCHAR(200) NOT NULL,
    sort_order   INT NOT NULL DEFAULT 0,
    UNIQUE (attribute_id, value)
);
CREATE TABLE IF NOT EXISTS product_attribute_values (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id   UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    variant_id   UUID REFERENCES product_variants(id) ON DELETE CASCADE,
    attribute_id UUID NOT NULL REFERENCES attribute_definitions(id),
    option_id    UUID REFERENCES attribute_options(id),
    value_text   TEXT,
    value_number NUMERIC,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- دقیقاً یک شکل مقدار معتبر
    CHECK ((option_id IS NOT NULL)::int + (value_text IS NOT NULL)::int + (value_number IS NOT NULL)::int = 1)
);
CREATE INDEX IF NOT EXISTS ix_pav_product  ON product_attribute_values (product_id, attribute_id);
CREATE INDEX IF NOT EXISTS ix_pav_variant  ON product_attribute_values (variant_id, attribute_id);

-- تصاویر/رسانه محصول (DB v2 §6.5 — دیتابیس فقط storage_key نگه می‌دارد)
CREATE TABLE IF NOT EXISTS product_media (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id  UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    variant_id  UUID REFERENCES product_variants(id) ON DELETE CASCADE,
    media_type  TEXT NOT NULL DEFAULT 'IMAGE' CHECK (media_type IN ('IMAGE','VIDEO','DOCUMENT')),
    storage_key TEXT NOT NULL,
    alt_text    TEXT,
    sort_order  INT NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_product_media_product ON product_media (product_id, sort_order);

CREATE TRIGGER trg_brands_updated    BEFORE UPDATE ON brands    FOR EACH ROW EXECUTE FUNCTION khabavar_set_updated_at();
CREATE TRIGGER trg_categories_updated BEFORE UPDATE ON categories FOR EACH ROW EXECUTE FUNCTION khabavar_set_updated_at();
CREATE TRIGGER trg_products_updated  BEFORE UPDATE ON products  FOR EACH ROW EXECUTE FUNCTION khabavar_set_updated_at();
CREATE TRIGGER trg_variants_updated  BEFORE UPDATE ON product_variants FOR EACH ROW EXECUTE FUNCTION khabavar_set_updated_at();

COMMIT;

=== 000003_supplier.up.sql ===
-- 000003_supplier.up.sql
-- ماژول Supplier — مطابق DB v2 §۷ + تاریخچه قیمت (قاعده §7.3) + نسخه سیاست ارزیابی (§7.4)

BEGIN;

CREATE TABLE IF NOT EXISTS suppliers (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name                VARCHAR(200) NOT NULL,
    supplier_type       TEXT NOT NULL,          -- واژگان انواع تأمین در Seed ثبت می‌شود (Seed §۱۷)
    registration_number VARCHAR(100),
    contact_name        VARCHAR(200),
    phone               VARCHAR(20),
    email               VARCHAR(255),
    address             TEXT,
    website             VARCHAR(255),
    status              TEXT NOT NULL DEFAULT 'ACTIVE'
                        CHECK (status IN ('ACTIVE','PROBATION','SUSPENDED','RETIRED')),  -- پیشنهاد واژگان
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS supplier_products (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    supplier_id             UUID NOT NULL REFERENCES suppliers(id),
    product_id              UUID NOT NULL REFERENCES products(id),
    supplier_product_code   VARCHAR(100),
    minimum_order_quantity  INT CHECK (minimum_order_quantity IS NULL OR minimum_order_quantity > 0),
    lead_time_days          INT CHECK (lead_time_days IS NULL OR lead_time_days >= 0),
    status                  TEXT NOT NULL DEFAULT 'ACTIVE'
                            CHECK (status IN ('ACTIVE','INACTIVE')),
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (supplier_id, product_id)        -- §۱۵: supplier_id, product_id
);

-- تأمین یک Variant از چند تأمین‌کننده با شرایط تجاری متفاوت (§7.3)
CREATE TABLE IF NOT EXISTS supplier_variants (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    supplier_product_id   UUID NOT NULL REFERENCES supplier_products(id) ON DELETE CASCADE,
    variant_id            UUID NOT NULL REFERENCES product_variants(id),
    supplier_sku          VARCHAR(100),
    purchase_price        BIGINT CHECK (purchase_price IS NULL OR purchase_price >= 0),  -- IRR
    currency              CHAR(3) NOT NULL DEFAULT 'IRR',
    available_quantity    INT NOT NULL DEFAULT 0 CHECK (available_quantity >= 0),
    last_stock_check_at   TIMESTAMPTZ,
    price_valid_until     TIMESTAMPTZ,        -- قیمت تأمین تاریخ اعتبار دارد (§7.3)
    last_price_check_at   TIMESTAMPTZ,
    status                TEXT NOT NULL DEFAULT 'ACTIVE'
                          CHECK (status IN ('ACTIVE','INACTIVE')),
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_supplier_variants_variant ON supplier_variants (variant_id);

-- تاریخچه قیمت تأمین‌کننده — قاعده §7.3: «هر تغییر قیمت باید در تاریخچه ثبت شود»
CREATE TABLE IF NOT EXISTS supplier_variant_price_history (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    supplier_variant_id UUID NOT NULL REFERENCES supplier_variants(id) ON DELETE CASCADE,
    purchase_price      BIGINT NOT NULL CHECK (purchase_price >= 0),
    currency            CHAR(3) NOT NULL DEFAULT 'IRR',
    valid_from          TIMESTAMPTZ NOT NULL DEFAULT now(),
    valid_until         TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_svp_history ON supplier_variant_price_history (supplier_variant_id, valid_from DESC);

-- ارزیابی وزنی تأمین‌کننده (Supplier Eval §۳-۴ + DB v2 §7.4)
CREATE TABLE IF NOT EXISTS supplier_evaluations (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    supplier_id         UUID NOT NULL REFERENCES suppliers(id),
    quality_score       INT NOT NULL CHECK (quality_score       BETWEEN 0 AND 100),
    pricing_score       INT NOT NULL CHECK (pricing_score       BETWEEN 0 AND 100),
    availability_score  INT NOT NULL CHECK (availability_score  BETWEEN 0 AND 100),
    warranty_score      INT NOT NULL CHECK (warranty_score      BETWEEN 0 AND 100),
    documentation_score INT NOT NULL CHECK (documentation_score BETWEEN 0 AND 100),
    flexibility_score   INT NOT NULL CHECK (flexibility_score   BETWEEN 0 AND 100),
    logistics_score     INT NOT NULL CHECK (logistics_score     BETWEEN 0 AND 100),
    communication_score INT NOT NULL CHECK (communication_score BETWEEN 0 AND 100),
    total_score         NUMERIC(5,2) NOT NULL CHECK (total_score BETWEEN 0 AND 100),
    evaluation_date     DATE NOT NULL,
    reviewer_id         UUID REFERENCES admin_users(id),   -- ثبت ارزیاب
    weights_version     VARCHAR(20) NOT NULL,  -- وزن‌ها در نسخه سیاست ثبت می‌شوند تا تاریخچه مخدوش نشود (§7.4)
    notes               TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_supplier_eval ON supplier_evaluations (supplier_id, evaluation_date DESC);

CREATE TRIGGER trg_suppliers_updated   BEFORE UPDATE ON suppliers   FOR EACH ROW EXECUTE FUNCTION khabavar_set_updated_at();
CREATE TRIGGER trg_sup_variants_update BEFORE UPDATE ON supplier_variants FOR EACH ROW EXECUTE FUNCTION khabavar_set_updated_at();

COMMIT;

=== 000004_inventory.up.sql ===
-- 000004_inventory.up.sql
-- ماژول Inventory — مطابق DB v2 §۸ (انبار، موجودی، دفتر حرکت، رزرو تراکنشی)

BEGIN;

CREATE TABLE IF NOT EXISTS warehouses (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name           VARCHAR(200) NOT NULL,
    warehouse_code VARCHAR(50) NOT NULL UNIQUE,
    address        TEXT,
    status         TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','INACTIVE')),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS stock_balances (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    warehouse_id      UUID NOT NULL REFERENCES warehouses(id),
    variant_id        UUID NOT NULL REFERENCES product_variants(id),
    on_hand_quantity  INT NOT NULL DEFAULT 0 CHECK (on_hand_quantity  >= 0),
    reserved_quantity INT NOT NULL DEFAULT 0 CHECK (reserved_quantity >= 0),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (warehouse_id, variant_id)     -- §۸.۲ قید یکتا / §۱۵
    -- قابل فروش = on_hand - reserved (در لایه Domain؛ هرگز منفی نمی‌شود)
);

-- دفتر ثبت تغییرات موجودی — رکوردها قابل حذف نیستند؛ اصلاح فقط با حرکت معکوس (§۸.۳)
CREATE TABLE IF NOT EXISTS stock_movements (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    warehouse_id   UUID NOT NULL REFERENCES warehouses(id),
    variant_id     UUID NOT NULL REFERENCES product_variants(id),
    movement_type  TEXT NOT NULL CHECK (movement_type IN
                   ('RECEIPT','SALE','RESERVATION','RELEASE','RETURN','ADJUSTMENT','DAMAGE','TRANSFER')),
    quantity       INT NOT NULL CHECK (quantity <> 0),
    reference_type TEXT,                   -- مثل ORDER / FULFILLMENT / MANUAL
    reference_id   UUID,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by     UUID REFERENCES admin_users(id)
);
CREATE INDEX IF NOT EXISTS ix_movements_variant ON stock_movements (warehouse_id, variant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS ix_movements_ref     ON stock_movements (reference_type, reference_id);

-- رزرو موجودی — منبع مشخص: انبار یا تأمین‌کننده (§۸.۴)
CREATE TABLE IF NOT EXISTS inventory_reservations (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_item_id  UUID NOT NULL,           -- FK در 000006_commerce پس از ایجاد جدول اضافه می‌شود
    variant_id     UUID NOT NULL REFERENCES product_variants(id),
    warehouse_id   UUID REFERENCES warehouses(id),
    supplier_id    UUID REFERENCES suppliers(id),
    quantity       INT NOT NULL CHECK (quantity > 0),
    status         TEXT NOT NULL DEFAULT 'ACTIVE'
                   CHECK (status IN ('ACTIVE','CONVERTED','RELEASED','EXPIRED')),  -- پیشنهاد واژگان
    expires_at     TIMESTAMPTZ NOT NULL,    -- رزرو بدون انقضا معنا ندارد (§۱۴.۲ / PRD O-4)
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- منبع رزرو باید مشخص باشد؛ برای موجودی تأمین‌کننده، تعیین تکلیف جداگانه در فرآیند تأیید است (§۸.۴)
    CONSTRAINT chk_reservation_source CHECK (
        (warehouse_id IS NOT NULL AND supplier_id IS NULL)
        OR (warehouse_id IS NULL AND supplier_id IS NOT NULL)
    )
);
CREATE INDEX IF NOT EXISTS ix_reservations_expiry ON inventory_reservations (status, expires_at);  -- §۱۵

COMMIT;

=== 000005_pricing.up.sql ===
-- 000005_pricing.up.sql
-- ماژول Pricing — مطابق DB v2 §۹ (هزینه تمام‌شده، قیمت فروش، Snapshot مالی)
-- نکته: FKهای ارجاع به orders/order_items در 000006_commerce اضافه می‌شوند (ترتیب Migration طبق DB v2 §۱۶)

BEGIN;

-- تاریخچه محاسبات هزینه تمام‌شده (§9.1) — مبالغ BIGINT ریال
CREATE TABLE IF NOT EXISTS product_cost_records (
    id                         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    variant_id                 UUID NOT NULL REFERENCES product_variants(id),
    supply_type                TEXT NOT NULL CHECK (supply_type IN ('OWNED_INVENTORY','SUPPLIER_SOURCED')),
    supplier_id                UUID REFERENCES suppliers(id),   -- برای تأمین بیرونی
    purchase_cost              BIGINT NOT NULL CHECK (purchase_cost >= 0),
    inbound_freight            BIGINT NOT NULL DEFAULT 0 CHECK (inbound_freight >= 0),
    inspection_cost            BIGINT NOT NULL DEFAULT 0 CHECK (inspection_cost >= 0),
    packaging_cost             BIGINT NOT NULL DEFAULT 0 CHECK (packaging_cost >= 0),
    fulfillment_cost           BIGINT NOT NULL DEFAULT 0 CHECK (fulfillment_cost >= 0),
    estimated_payment_fee      BIGINT NOT NULL DEFAULT 0 CHECK (estimated_payment_fee >= 0),
    estimated_shipping_subsidy BIGINT NOT NULL DEFAULT 0 CHECK (estimated_shipping_subsidy >= 0),
    expected_return_cost       BIGINT NOT NULL DEFAULT 0 CHECK (expected_return_cost >= 0),
    currency                   CHAR(3) NOT NULL DEFAULT 'IRR',
    valid_from                 TIMESTAMPTZ NOT NULL,
    valid_until                TIMESTAMPTZ,
    created_at                 TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by                 UUID REFERENCES admin_users(id)
);
CREATE INDEX IF NOT EXISTS ix_cost_records ON product_cost_records (variant_id, valid_from DESC);  -- §۱۵

-- قیمت فروش با تأیید نقش مجاز (§9.2) — سود مشارکت می‌تواند صفر/منفی شود (BR-06: هشدار و بازبینی)
CREATE TABLE IF NOT EXISTS product_price_records (
    id                        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    variant_id                UUID NOT NULL REFERENCES product_variants(id),
    selling_price             BIGINT NOT NULL CHECK (selling_price >= 0),
    discount_policy           JSONB,
    target_contribution_rate  NUMERIC(5,4),
    calculated_contribution   BIGINT,           -- بدون قید ≥ 0؛ مقدار ≤ 0 در لایه کاربرد هشدار می‌دهد
    currency                  CHAR(3) NOT NULL DEFAULT 'IRR',
    valid_from                TIMESTAMPTZ NOT NULL,
    valid_until               TIMESTAMPTZ,
    approved_by               UUID REFERENCES admin_users(id),   -- «قیمت فروش باید قابل تأیید توسط نقش مجاز باشد»
    created_at                TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_price_records ON product_price_records (variant_id, valid_from DESC);  -- §۱۵

-- Snapshot مالی سفارش — مبنای گزارش سودآوری سفارش‌های گذشته (§9.3)
-- هزینه‌های سطح Order با قاعده تخصیص مشخص میان اقلام تقسیم می‌شوند (روش در calculation_version ثبت است)
CREATE TABLE IF NOT EXISTS order_financial_snapshots (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id             UUID NOT NULL,      -- FK در 000006_commerce
    order_item_id        UUID,               -- NULL = ردیف سطح سفارش (هزینه مشترک)
    net_revenue          BIGINT NOT NULL CHECK (net_revenue >= 0),
    cogs                 BIGINT NOT NULL DEFAULT 0 CHECK (cogs >= 0),
    variable_costs       BIGINT NOT NULL DEFAULT 0 CHECK (variable_costs >= 0),
    contribution         BIGINT NOT NULL,    -- می‌تواند منفی باشد
    contribution_rate    NUMERIC(7,4),
    calculation_version  VARCHAR(20) NOT NULL,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_fin_snapshots_order ON order_financial_snapshots (order_id);

COMMIT;

=== 000006_commerce.up.sql ===
-- 000006_commerce.up.sql
-- ماژول Commerce — مطابق DB v2 §۱۰ + ADR-COM-001 (یک سفارش/یک پرداخت/چند مرسوله)
-- + ADR-ARCH-008 (Idempotency) + MA-8 (خرید مهمان) + اتصال FKهای معلق 000004/000005

BEGIN;

-- سبدها (§10.1) — سبد مهمان با session_token
CREATE TABLE IF NOT EXISTS carts (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id   UUID REFERENCES customers(id) ON DELETE SET NULL,
    session_token VARCHAR(100),            -- سبد مهمان (MA-8)
    status        TEXT NOT NULL DEFAULT 'ACTIVE'
                  CHECK (status IN ('ACTIVE','CONVERTED','ABANDONED','EXPIRED')),  -- پیشنهاد واژگان
    currency      CHAR(3) NOT NULL DEFAULT 'IRR',
    expires_at    TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_cart_owner CHECK (customer_id IS NOT NULL OR session_token IS NOT NULL)
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_carts_active_session ON carts (session_token) WHERE status = 'ACTIVE';
CREATE INDEX IF NOT EXISTS ix_carts_customer ON carts (customer_id, status);

CREATE TABLE IF NOT EXISTS cart_items (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    cart_id    UUID NOT NULL REFERENCES carts(id) ON DELETE CASCADE,
    variant_id UUID NOT NULL REFERENCES product_variants(id),
    quantity   INT NOT NULL CHECK (quantity > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (cart_id, variant_id)
);

-- سفارش‌ها (§10.3) — Snapshot آدرس؛ مبالغ IRR؛ یکتایی شماره تجاری
CREATE TABLE IF NOT EXISTS orders (
    id                        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_number              VARCHAR(50) NOT NULL UNIQUE,   -- مثل KHA-ORD-2026-000001 (§4.1) / §۱۵
    customer_id               UUID REFERENCES customers(id), -- NULL = خرید مهمان (MA-8)
    guest_phone               VARCHAR(20),                   -- پیگیری سفارش مهمان با شماره تماس (UJ-01)
    status                    TEXT NOT NULL DEFAULT 'PENDING_PAYMENT'
                              CHECK (status IN
                              ('PENDING_PAYMENT','PAID','IN_FULFILLMENT','PARTIALLY_SHIPPED',
                               'SHIPPED','DELIVERED','COMPLETED','CANCELLED','CLOSED')),  -- پیشنهاد واژگان
    currency                  CHAR(3) NOT NULL DEFAULT 'IRR',
    subtotal                  BIGINT NOT NULL CHECK (subtotal >= 0),
    discount_total            BIGINT NOT NULL DEFAULT 0 CHECK (discount_total >= 0),
    shipping_total            BIGINT NOT NULL DEFAULT 0 CHECK (shipping_total >= 0),
    tax_total                 BIGINT NOT NULL DEFAULT 0 CHECK (tax_total >= 0),
    grand_total               BIGINT NOT NULL CHECK (grand_total >= 0),
    paid_total                BIGINT NOT NULL DEFAULT 0 CHECK (paid_total >= 0),
    refunded_total            BIGINT NOT NULL DEFAULT 0 CHECK (refunded_total >= 0),
    shipping_address_snapshot JSONB NOT NULL,   -- آدرس هنگام سفارش Snapshot می‌شود (§5.2/ADR-DATA-003)
    created_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_orders_contact CHECK (customer_id IS NOT NULL OR guest_phone IS NOT NULL),
    -- سازگاری مبالغ سفارش (§10.3): grand = subtotal - discount + shipping + tax
    CONSTRAINT chk_orders_totals CHECK (
        grand_total = subtotal - discount_total + shipping_total + tax_total
    )
);
CREATE INDEX IF NOT EXISTS ix_orders_customer ON orders (customer_id, created_at DESC);  -- §۱۵
CREATE INDEX IF NOT EXISTS ix_orders_guest_phone ON orders (guest_phone) WHERE customer_id IS NULL;

CREATE TABLE IF NOT EXISTS order_items (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id             UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,   -- §۱۵
    product_id           UUID NOT NULL REFERENCES products(id),
    variant_id           UUID NOT NULL REFERENCES product_variants(id),
    sku_snapshot         VARCHAR(100) NOT NULL,   -- Snapshot مستقل از تغییرات کاتالوگ (§10.4)
    title_snapshot       VARCHAR(255) NOT NULL,
    attributes_snapshot  JSONB,
    quantity             INT NOT NULL CHECK (quantity > 0),
    unit_price           BIGINT NOT NULL CHECK (unit_price >= 0),
    discount_amount      BIGINT NOT NULL DEFAULT 0 CHECK (discount_amount >= 0),
    tax_amount           BIGINT NOT NULL DEFAULT 0 CHECK (tax_amount >= 0),
    line_total           BIGINT NOT NULL CHECK (line_total >= 0),
    supply_type_snapshot TEXT NOT NULL CHECK (supply_type_snapshot IN ('OWNED_INVENTORY','SUPPLIER_SOURCED')),
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_items_line_total CHECK (
        line_total = quantity * unit_price - discount_amount + tax_amount
    )
);
CREATE INDEX IF NOT EXISTS ix_order_items_variant ON order_items (variant_id);

-- پرداخت: تفکیک Intent و Attempt (§10.5)
CREATE TABLE IF NOT EXISTS payment_intents (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id   UUID NOT NULL REFERENCES orders(id),
    amount     BIGINT NOT NULL CHECK (amount > 0),
    currency   CHAR(3) NOT NULL DEFAULT 'IRR',
    status     TEXT NOT NULL DEFAULT 'REQUIRES_PAYMENT'
               CHECK (status IN ('REQUIRES_PAYMENT','PROCESSING','SUCCEEDED','FAILED','EXPIRED','CANCELLED')),
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- ADR-COM-001: فقط یک پرداخت موفق اصلی به ازای هر سفارش (§10.5 قید تجاری)
CREATE UNIQUE INDEX IF NOT EXISTS uq_intent_success_per_order
    ON payment_intents (order_id) WHERE status = 'SUCCEEDED';
-- فقط یک Intent فعال هم‌زمان برای هر سفارش
CREATE UNIQUE INDEX IF NOT EXISTS uq_intent_active_per_order
    ON payment_intents (order_id) WHERE status IN ('REQUIRES_PAYMENT','PROCESSING');

CREATE TABLE IF NOT EXISTS payment_attempts (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    payment_intent_id  UUID NOT NULL REFERENCES payment_intents(id),
    provider           TEXT NOT NULL,
    provider_reference TEXT,
    amount             BIGINT NOT NULL CHECK (amount > 0),
    status             TEXT NOT NULL DEFAULT 'INITIATED'
                       CHECK (status IN ('INITIATED','REDIRECTED','UNKNOWN','SUCCEEDED','FAILED')),
    failure_code       TEXT,
    idempotency_key    VARCHAR(100) UNIQUE,    -- ADR-ARCH-008: عملیات تکراری اثر تکراری ندارد
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at       TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS ix_attempts_intent ON payment_attempts (payment_intent_id, created_at DESC);

-- بازپرداخت جزئی (§10.6) — اتصال به تراکنش موفق اصلی؛ سقف بازپرداخت در لایه کاربرد کنترل می‌شود (BR-05)
CREATE TABLE IF NOT EXISTS refunds (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id           UUID NOT NULL REFERENCES orders(id),
    payment_attempt_id UUID NOT NULL REFERENCES payment_attempts(id),  -- باید SUCCEEDED باشد (کنترل لایه کاربرد)
    order_item_id      UUID REFERENCES order_items(id),                -- NULL = بازپرداخت سطح سفارش
    amount             BIGINT NOT NULL CHECK (amount > 0),
    reason             TEXT NOT NULL,
    status             TEXT NOT NULL DEFAULT 'REQUESTED'
                       CHECK (status IN ('REQUESTED','APPROVED','PROCESSING','COMPLETED','REJECTED','FAILED')),
    provider_reference TEXT,
    approved_by        UUID REFERENCES admin_users(id),
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at       TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS ix_refunds_order ON refunds (order_id, created_at DESC);

-- ── اتصال FKهای معلق ماژول‌های قبلی (ترتیب Migration طبق DB v2 §۱۶) ──
ALTER TABLE inventory_reservations
    ADD CONSTRAINT fk_reservations_order_item
    FOREIGN KEY (order_item_id) REFERENCES order_items(id) ON DELETE RESTRICT;
ALTER TABLE order_financial_snapshots
    ADD CONSTRAINT fk_fin_snapshots_order
    FOREIGN KEY (order_id) REFERENCES orders(id) ON DELETE RESTRICT;
ALTER TABLE order_financial_snapshots
    ADD CONSTRAINT fk_fin_snapshots_order_item
    FOREIGN KEY (order_item_id) REFERENCES order_items(id) ON DELETE RESTRICT;

CREATE TRIGGER trg_carts_updated    BEFORE UPDATE ON carts    FOR EACH ROW EXECUTE FUNCTION khabavar_set_updated_at();
CREATE TRIGGER trg_cart_items_upd   BEFORE UPDATE ON cart_items FOR EACH ROW EXECUTE FUNCTION khabavar_set_updated_at();
CREATE TRIGGER trg_orders_updated   BEFORE UPDATE ON orders   FOR EACH ROW EXECUTE FUNCTION khabavar_set_updated_at();

COMMIT;

=== 000007_fulfillment.up.sql ===
-- 000007_fulfillment.up.sql
-- ماژول Fulfillment — مطابق DB v2 §۱۱ + ADR-COM-001 (تفکیک Fulfillment/مرسوله، رهگیری مستقل)

BEGIN;

CREATE TABLE IF NOT EXISTS fulfillment_orders (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id              UUID NOT NULL REFERENCES orders(id),
    source_type           TEXT NOT NULL CHECK (source_type IN ('OWNED_INVENTORY','SUPPLIER_SOURCED')),  -- §11.1
    warehouse_id          UUID REFERENCES warehouses(id),   -- برای تأمین از انبار خود
    supplier_id           UUID REFERENCES suppliers(id),    -- برای تأمین بیرونی
    status                TEXT NOT NULL DEFAULT 'PENDING'
                          CHECK (status IN ('PENDING','CONFIRMED','PROCESSING','READY','SHIPPED',
                                            'DELIVERED','CANCELLED','REJECTED')),  -- پیشنهاد واژگان
    estimated_ready_at    TIMESTAMPTZ,
    estimated_delivery_at TIMESTAMPTZ,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- منبع Fulfillment باید با نوع تأمین سازگار باشد (§11.1)
    CONSTRAINT chk_fulfillment_source CHECK (
        (source_type = 'OWNED_INVENTORY' AND warehouse_id IS NOT NULL AND supplier_id IS NULL)
        OR (source_type = 'SUPPLIER_SOURCED' AND supplier_id IS NOT NULL AND warehouse_id IS NULL)
    )
);
CREATE INDEX IF NOT EXISTS ix_fulfillment_orders ON fulfillment_orders (order_id, status);  -- §۱۵
CREATE INDEX IF NOT EXISTS ix_fulfillment_supplier ON fulfillment_orders (supplier_id, status);

CREATE TABLE IF NOT EXISTS fulfillment_items (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    fulfillment_order_id UUID NOT NULL REFERENCES fulfillment_orders(id) ON DELETE CASCADE,
    order_item_id        UUID NOT NULL REFERENCES order_items(id),
    quantity             INT NOT NULL CHECK (quantity > 0),
    status               TEXT NOT NULL DEFAULT 'PENDING'
                         CHECK (status IN ('PENDING','CONFIRMED','PROCESSING','SHIPPED','DELIVERED','CANCELLED','REJECTED')),
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now()
    -- قید §11.2: مجموع تخصیص به Fulfillmentها ≤ تعداد قلم سفارش
    -- (کنترل در تراکنش لایه کاربرد با قفل ردیفی — §11.2/§14)
);
CREATE INDEX IF NOT EXISTS ix_fulfillment_items_item ON fulfillment_items (order_item_id);

-- هر Fulfillment می‌تواند چند Shipment داشته باشد (§11.3)
CREATE TABLE IF NOT EXISTS shipments (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    fulfillment_order_id UUID NOT NULL REFERENCES fulfillment_orders(id) ON DELETE CASCADE,
    carrier              TEXT NOT NULL,
    service_type         TEXT,
    tracking_number      VARCHAR(100),
    shipping_cost        BIGINT NOT NULL DEFAULT 0 CHECK (shipping_cost >= 0),
    status               TEXT NOT NULL DEFAULT 'LABEL_CREATED'
                         CHECK (status IN ('LABEL_CREATED','IN_TRANSIT','OUT_FOR_DELIVERY',
                                           'DELIVERED','FAILED','RETURNED')),  -- پیشنهاد واژگان
    shipped_at           TIMESTAMPTZ,
    delivered_at         TIMESTAMPTZ,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_shipments_tracking ON shipments (tracking_number);  -- §۱۵
CREATE INDEX IF NOT EXISTS ix_shipments_fulfillment ON shipments (fulfillment_order_id, status);

-- یک قلم سفارش می‌تواند در چند بسته ارسال شود (§11.4)
CREATE TABLE IF NOT EXISTS shipment_items (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    shipment_id         UUID NOT NULL REFERENCES shipments(id) ON DELETE CASCADE,
    fulfillment_item_id UUID NOT NULL REFERENCES fulfillment_items(id),
    quantity            INT NOT NULL CHECK (quantity > 0)
);

-- رویدادهای رهگیری — ثبت ترتیب زمانی (§11.5)
CREATE TABLE IF NOT EXISTS shipment_events (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    shipment_id        UUID NOT NULL REFERENCES shipments(id) ON DELETE CASCADE,
    event_code         TEXT NOT NULL,
    description        TEXT,
    occurred_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    source             TEXT NOT NULL DEFAULT 'MANUAL'
                       CHECK (source IN ('MANUAL','CARRIER_API','WEBHOOK')),  -- پیشنهاد واژگان
    external_reference TEXT,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_shipment_events ON shipment_events (shipment_id, occurred_at);  -- §۱۵

CREATE TRIGGER trg_fulfillment_updated BEFORE UPDATE ON fulfillment_orders FOR EACH ROW EXECUTE FUNCTION khabavar_set_updated_at();

COMMIT;

=== 000008_content.up.sql ===
-- 000008_content.up.sql
-- ماژول Content — مطابق DB v2 §۱۲ + گردش کار تحریریه (SAD §۷.۲) + ADR-CONTENT-001

BEGIN;

CREATE TABLE IF NOT EXISTS topics (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       VARCHAR(200) NOT NULL,
    slug       VARCHAR(255) NOT NULL UNIQUE,
    parent_id  UUID REFERENCES topics(id),
    sort_order INT NOT NULL DEFAULT 0,
    status     TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','INACTIVE')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- ۱۲ موضوع پایه در Seed (DB v2 §12.2 / SAD §7.1)

CREATE TABLE IF NOT EXISTS articles (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title             VARCHAR(300) NOT NULL,
    slug              VARCHAR(255) NOT NULL UNIQUE,  -- §۱۵
    summary           TEXT,
    body              TEXT NOT NULL,
    language          VARCHAR(10) NOT NULL DEFAULT 'fa-IR',
    source_type       TEXT NOT NULL DEFAULT 'ORIGINAL'
                      CHECK (source_type IN ('ORIGINAL','TRANSLATION','CURATED')),  -- SAD §7.3
    source_url        TEXT,
    parent_article_id UUID REFERENCES articles(id),  -- نسخه‌های زبانی → مقاله مبدأ (SAD §7.2 ترجمه)
    author_id         UUID REFERENCES admin_users(id),
    editor_id         UUID REFERENCES admin_users(id),
    status            TEXT NOT NULL DEFAULT 'DRAFT'
                      CHECK (status IN ('DRAFT','IN_REVIEW','APPROVED','SCHEDULED',
                                        'PUBLISHED','UNPUBLISHED','ARCHIVED')),  -- SAD §7.2
    published_at      TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_articles_status ON articles (status, published_at DESC);  -- §۱۵

-- رابطه مقاله-موضوع (چندبه‌چند)
CREATE TABLE IF NOT EXISTS article_topics (
    article_id UUID NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
    topic_id   UUID NOT NULL REFERENCES topics(id) ON DELETE CASCADE,
    PRIMARY KEY (article_id, topic_id)
);

-- منابع علمی مقاله — ADR-CONTENT-001: مقاله بدون منبع معتبر قابل انتشار نیست
-- (کنترل «انتشار فقط با ≥۱ منبع» در گردش کار لایه کاربرد)
CREATE TABLE IF NOT EXISTS article_sources (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    article_id  UUID NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
    source_type TEXT NOT NULL CHECK (source_type IN ('JOURNAL','BOOK','GUIDELINE','EXPERT','OTHER')),
    citation    TEXT NOT NULL,
    url         TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_article_sources ON article_sources (article_id);

-- رابطه چندبه‌چند مقاله-محصول (§12.3) با دلیل ارتباط (PRD MA-5)
CREATE TABLE IF NOT EXISTS product_article_relations (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id    UUID NOT NULL REFERENCES products(id),
    article_id    UUID NOT NULL REFERENCES articles(id),
    relation_type TEXT NOT NULL CHECK (relation_type IN
                  ('RELATED','EDUCATIONAL','PRODUCT_GUIDE','COMPARISON')),   -- §12.3 عیناً
    relation_reason TEXT NOT NULL,      -- MA-5: «دلیل ارتباط» ثبت‌شده، نه تبلیغ صرف
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (product_id, article_id, relation_type)
);
CREATE INDEX IF NOT EXISTS ix_par_article ON product_article_relations (article_id);

CREATE TRIGGER trg_articles_updated BEFORE UPDATE ON articles FOR EACH ROW EXECUTE FUNCTION khabavar_set_updated_at();

COMMIT;

=== 000009_audit.up.sql ===
-- 000009_audit.up.sql
-- ماژول Audit — مطابق DB v2 §۱۳ + ADR-ARCH-007 (Transactional Outbox) + ADR-ARCH-010

BEGIN;

-- Audit Log عملیات حساس (§13.1): قیمت، هزینه خرید، تأیید تأمین، موجودی، بازپرداخت، وضعیت سفارش، شرایط ارسال
-- Append-only: بدون FK تا حذف موجودیت، ردپای ممیزی را از بین نبرد؛ دستکاری عادی ممنوع (§۱۸)
CREATE TABLE IF NOT EXISTS audit_logs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_id    UUID,                      -- شناسه کاربر/ادمین/سیستم بدون FK
    actor_type  TEXT NOT NULL CHECK (actor_type IN ('CUSTOMER','ADMIN','SYSTEM')),
    action      TEXT NOT NULL,             -- مثل price.record.approve
    entity_type TEXT NOT NULL,
    entity_id   UUID,
    old_values  JSONB,
    new_values  JSONB,
    ip_address  INET,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_audit_entity ON audit_logs (entity_type, entity_id, created_at DESC);
CREATE INDEX IF NOT EXISTS ix_audit_actor  ON audit_logs (actor_id, created_at DESC);

-- Domain Events / Transactional Outbox (§13.2) — تأیید پرداخت و ثبت رویداد در یک تراکنش (§14.3)
CREATE TABLE IF NOT EXISTS domain_events (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    aggregate_type TEXT NOT NULL,          -- مثل Order / Payment / Shipment
    aggregate_id   UUID NOT NULL,
    event_type     TEXT NOT NULL,          -- مثل OrderConfirmed (Commerce §۱۰)
    payload        JSONB,
    occurred_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at   TIMESTAMPTZ,
    retry_count    INT NOT NULL DEFAULT 0
);
-- صف Outbox: مصرف‌کننده فقط رویدادهای منتشرنشده را برمی‌دارد
CREATE INDEX IF NOT EXISTS ix_domain_events_unpublished
    ON domain_events (occurred_at) WHERE published_at IS NULL;
CREATE INDEX IF NOT EXISTS ix_domain_events_aggregate
    ON domain_events (aggregate_type, aggregate_id);

COMMIT;

=== 000010_db_roles.up.sql ===
-- 000010_db_roles.up.sql — بخشی از طرح اصلی؛ وابستگی سخت DEP v1.1 §۴ (نقش‌های DB) — اجرا توسط اپراتور زیرساخت
-- نقش‌های دیتابیس طبق DB v2 §۱۸ (حداقل‌دسترسی؛ رمزها خارج از مخزن تنظیم می‌شوند)
-- در محیط‌های مدیریت‌شده ممکن است ایجاد نقش نیازمند نقش superuser باشد؛ این فایل را اپراتور زیرساخت اجرا می‌کند.

BEGIN;
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'db_migrator') THEN
        CREATE ROLE db_migrator NOLOGIN;         -- اجرای Migration
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'app_runtime') THEN
        CREATE ROLE app_runtime NOLOGIN;         -- عملیات برنامه — دسترسی حداقل
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'reporting_reader') THEN
        CREATE ROLE reporting_reader NOLOGIN;    -- خواندن داده‌های گزارش
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'backup_operator') THEN
        CREATE ROLE backup_operator NOLOGIN;     -- عملیات پشتیبان‌گیری
    END IF;
END
$$;

-- دسترسی‌های پایه (در استقرار نهایی مطابق محیط تنظیم می‌شود)
GRANT USAGE ON SCHEMA public TO app_runtime, reporting_reader;
GRANT SELECT ON ALL TABLES IN SCHEMA public TO reporting_reader;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON TABLES TO reporting_reader;

COMMIT;

