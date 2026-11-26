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
