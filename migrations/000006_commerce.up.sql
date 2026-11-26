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
