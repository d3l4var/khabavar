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
