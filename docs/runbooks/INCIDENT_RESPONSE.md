# Runbook: Incident Response (R-18 §۶)

آخرین به‌روزرسانی: 2026-11-26

## هدف

این ران‌بوک فرآیند پاسخ به حوادث (incidents) تولید khabavar را تعریف می‌کند — از تشخیص تا رفع، ارتباط با ذی‌نفعان، و postmortem.

## سطوح شدت (Severity)

| سطح | تعریف | زمان پاسخ | مثال |
|------|-------|----------|------|
| **S1 — بحرانی** | سرویس کاملاً از دسترس خارج است یا داده از بین رفته | ۱۵ دقیقه | API ۵۰۰ برمی‌گرداند، دیتابیس down |
| **S2 — جدی** | بخش عمده‌ای از سرویس خراب است | ۳۰ دقیقه | پرداخت کار نمی‌کند، ورود کار نمی‌کند |
| **S3 — متوسط** | یک قابلیت خراب است، بقیه سالم | ۲ ساعت | جستجو نتیجه نمی‌دهد، یک endpoint ۵۰۰ |
| **S4 — خفیف** | مشکل جزئی بدون تأثیر کاربر | ۲۴ ساعت | آهسته‌بودن یک گزارش، هشدار نویز |

## فرآیند پاسخ

### ۱. تشخیص (Detection)

منابع تشخیص:
- **Health checks**: `https://khabavar.ir/healthz` (هر ۳۰ ثانیه توسط uptime monitor)
- **Alerting rules** (Prometheus → AlertManager):
  - `rate(http_requests_total{status=~"5.."}[5m]) > 0.1` → S2
  - `up{khabavar-api} == 0 for 1m` → S1
  - `pg_connections_used / pg_connections_max > 0.8` → S3
- **گزارش کاربر**: ایمیل/تلگرام ops@khabavar.ir

### ۲. Triage (۱۵ دقیقه برای S1/S2)

1. **تأیید severity**: آیا واقعاً S1 است؟ (چند کاربر تأثیر می‌بینند؟)
2. **تعیین Incident Commander**: اولین پاسخ‌دهنده مسئول است تا relief برسد.
3. **اعلان**: برای S1/S2، در کانال `#khabavar-incidents` تلگرام اعلام کن:
   ```
   [S1] 2026-11-26 14:30 — API 500 errors spiking
   IC: @name
   Status: investigating
   ```

### ۳. Investigation

1. **لاگ‌ها را بررسی کن**:
   ```bash
   journalctl -u khabavar-api --since "10 min ago" | grep -i error
   ```
2. **متریک‌ها را بررسی کن**: Grafana dashboard khabavar
3. **آخرین تغییر**: آیا استقرار اخیر بوده؟ migration؟ تغییر nginx؟
4. **فرضیه بساز و تست کن** — هر فرضیه را در کانال ثبت کن.

### ۴. Mitigation (رفع موقت)

هدف اول: برگرداندن سرویس، نه ریشه‌یابی.
- **Rollback**: اگر آخرین استقرار علت است، باینری قبلی را بازگردان (`docs/runbooks/DEPLOYMENT.md`).
- **Restart**: `systemctl restart khabavar-api`
- **Scale**: اگر DB connection کم است، `KH_DB_MAX_OPEN_CONNS` را افزایش بده.
- **Failover**: اگر سرور down است، روی سرور standby استقرار کن.

### ۵. Resolution

1. ریشه را اصلاح کن (patch، migration، config).
2. در staging تست کن.
3. به production استقرار بده.
4. در کانال اعلام کن: `[RESOLVED] ... — root cause: ...`

### ۶. Postmortem (در عرض ۴۸ ساعت)

سند بدون سرزنش که توضیح می‌دهد:
- **چه اتفاقی افتاد** (timeline دقیق)
- **تأثیر** (مدت downtime، کاربران تأثیرگذار، سفارش‌های از‌دست‌رفته)
- **علت ریشه‌ای** (۵-why analysis)
- **اقدامات پیشگیرانه** (با مالک و مهلت)

قالب: `docs/postmortems/YYYYMMDD-incident-title.md`

## حوادث شایع و راه‌حل

### API ۵۰۰ برمی‌گرداند

1. `journalctl -u khabavar-api --since "5 min ago" | grep -i error`
2. اگر `connection refused` → DB down: `systemctl status postgresql`
3. اگر `too many connections` → pool پر است: restart api
4. اگر panic در یک handler → باینری را rollback کن

### ورود کار نمی‌کند (OTP)

1. identity unit را بررسی: `systemctl status khabavar-identity`
2. آیا SMS adapter کار می‌کند؟ (در dev mock، در prod Kavenegar)
3. rate limit را بررسی: `KH_OTP_MAX_PER_HOUR` — آیا IP مسدود شده؟

### پرداخت ناموفق

1. لاگ commerce: `journalctl -u khabavar-api | grep -i payment`
2. آیا payment intent در حالت `FAILED` است؟ → `ReconcileTransaction` را اجرا کن
3. اگر مبلغ برداشت شده ولی سفارش `PENDING_PAYMENT` → دستی `ConfirmPaymentForOrder` را از admin صدا بزن

### DB دیسک پر

1. `df -h /var/lib/postgresql`
2. WAL archive‌ها را پاک‌سازی کن (با احتیاط)
3. backup‌های قدیمی weekly را压缩 کن
4. اگر فوری: `VACUUM FULL` در ساعات کم‌ترافیک

## ارتباط با ذی‌نفعان

- **S1/S2**: به مالک پروژه در عرض ۳۰ دقیقه اطلاع بده.
- **به‌روزرسانی هر ۳۰ دقیقه** در کانال تا resolution.
- **پس از resolution**: خلاصه برای مالک + تیم در عرض ۲۴ ساعت.
