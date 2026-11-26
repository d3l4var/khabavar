-- 000001_identity.down.sql
-- سیاست: Migration فقط forward است (DEP v1.1 §۶، TS §۶).
-- down فقط اضطراری با workflow_dispatch emergency_rollback_migration و تأیید اپراتور اجرا می‌شود.
-- این فایل عمداً خالی است؛ در صورت نیاز، دستور معکوس سازگار اینجا نوشته شود.
BEGIN;
COMMIT;
