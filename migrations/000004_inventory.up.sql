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
