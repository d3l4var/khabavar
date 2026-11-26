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
