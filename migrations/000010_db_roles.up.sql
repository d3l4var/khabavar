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
