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
