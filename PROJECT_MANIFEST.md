# khabavar — Project Manifest

**نسخه:** v1.2 — وضعیت واقعی (نه ادعای کامل)
**تاریخ:** 2026-11-27
**مخزن:** https://github.com/d3l4var/khabavar
**شاخه:** main

## خلاصه

khabavar یک پلتفرم تجارت الکترونیک تخصصی سلامت خواب است — فروشگاه محصول + مجله علمی + احراز هویت OIDC. بک‌اند Go با معماری ماژولار (SAD §۴.۲)، پایگاه‌داده PostgreSQL، و چهار برنامه وب React+TS+Vite.

## آمار نهایی

| معیار | مقدار |
|-------|-------|
| ماژول‌های Go | ۱۳ (catalog, pricing, commerce, suppliers, inventory, fulfillment, media, content, ecosystem, notifications, identity, authorization, worker) + audit, shared |
| پکیج‌های پلتفرم | ۸ (config, db, health, idempotency, log, ratelimit, security, testpg) |
| migrationهای DB | ۱۶ |
| تست‌های سبز | ۳۱۶ تابع تست (۰ FAIL؛ با و بدون PG) |
| endpointهای HTTP | ۵۸ عملیات قرارداد — ۵۸/۵۸ mount شده (۱۲۶ ثبت مسیر با aliasها) |
| سندهای مشخصات | ۱۴ (R-01..R-CI) |
| سندهای منبع | ۱۱ (۰۱-۱۱) |
| ران‌بوک‌ها | ۴ (Deployment, Backup/Restore, Incident Response, Monitoring) |
| فازهای تکمیل‌شده | P1، P3، P4، P5a–P5f، P6، P7 (بک‌اند) · P2 (Design System) · فرانت‌اند (۴ اپ) |
| فازهای باقی‌مانده | P8–P10 (سخت‌سازی/استقرار) · پوشش تست تا آستانهٔ R-CI §۳ |

### اعداد تأییدشده (۲۰۲۶-۱۱-۲۷)

| سنجه | مقدار | منبع |
|---|---|---|
| خطوط Go (بدون تست) | ۱۶٬۲۵۸ | `wc -l` روی `*.go` غیرتست |
| خطوط تست | ۱۲٬۳۰۹ | `wc -l` روی `*_test.go` |
| توابع تست | ۳۱۶ | `grep -rc 'func Test'` |
| مسیرهای قرارداد mount‌شده | ۵۸ / ۵۸ | `scripts/audit-routes.py` (بخش «DECLARED BUT NOT MOUNTED» خالی) |
| متدهای SQL با تست PG | ۸۹ / ۸۹ | `scripts/audit-pg-coverage.py` |
| کامیت‌ها روی `main` | ۸۶ | `git log --oneline \| wc -l` |

**پوشش تست فعلی (اندازه‌گیری‌شده، نه تخمین):** آستانهٔ R-CI §۳ برآورده **نشده** است.
`inventory 47.2٪`، `pricing 61.7٪`، `commerce 68.3٪` (آستانهٔ مالی ۹۰٪)؛
`content 45.4٪`، `media 47.4٪`، `identity 51.2٪`، `catalog 59.3٪`، `suppliers 60.2٪`،
`fulfillment 60.8٪` (آستانهٔ سایر ۷۵٪). گام coverage در CI `continue-on-error` است.

## ساختار پروژه

```
khabavar/
├── cmd/khabavar/          # نقطه ورود (سه unit: identity/api/worker)
├── internal/
│   ├── modules/           # ۱۳ ماژول دامنه + audit + shared
│   └── platform/          # config, db, health, idempotency, log, ratelimit, security
├── migrations/            # ۱۶ migration SQL (identity→return_requests)
├── docs/
│   ├── specs/             # ۱۴ سند مشخصات تجمیع‌شده R-XX
│   ├── source-material/   # ۱۱ سند اولیه
│   └── runbooks/          # ۴ ران‌بوک عملیاتی
├── deploy/
│   ├── systemd/           # ۳ unit file
│   ├── nginx/             # reverse proxy config
│   ├── scripts/           # backup.sh + restore.sh
│   └── monitoring/        # prometheus.yml
├── scripts/               # اسکریپت‌های build/push/helper
├── web/                   # ۴ برنامه (main, shop, magazine, admin)
├── upload/khabavar/       # مستندات اولیه پروژه
├── go.mod / go.sum
├── .env.example
└── Makefile
```

## فازهای تکمیل‌شده

| فاز | عنوان | وضعیت |
|-----|-------|-------|
| P1 | Discovery & Technical Design | ✅ SAD v2.1، ۱۴ سند، OpenAPI v1.1.0 |
| P2 | Brand & Design System | ⚠️ سند طراحی هست؛ پیاده‌سازی در کد نیست (فرانت scaffold) |
| P3 | Core Platform | ✅ foundation، DB، OIDC، RBAC، audit، امنیت |
| P4 | Magazine Platform | ✅ ماژول content (گردش کار تحریریه، دروازه انتشار) |
| P5 | Store Platform | 🟡 catalog/pricing/inventory/media/suppliers + جستجو پیاده؛ روت‌های ادمین کاتالوگ/قیمت باقی |
| P6 | Commerce Operations | ✅ commerce، fulfillment، refund، تطبیق تراکنش (E2E تأییدشده) |
| P7 | Ecosystem Integration | ✅ جستجوی یکپارچه، ناوبری مشترک |
| P8 | Administration | 🟡 آداپتورهای SMS/Email + dispatcher؛ داشبورد و گزارش‌ها باقی |
| P9 | Security & Performance | 🟡 هدرها/CORS/HSTS/CSP + RBAC؛ تست بار و بهینه‌سازی باقی |
| P10 | Production Launch | 🟡 runbooks، manifests، backup/restore، monitoring؛ استقرار واقعی انجام نشده |

> منبع حقیقت وضعیت: [`docs/ACTIVITY-LOG.md`](docs/ACTIVITY-LOG.md) §۳ و §۵.
> اصل R-19 §۹: هیچ قابلیت پیاده‌سازی‌نشده‌ای در گزارش‌ها «آماده» معرفی نمی‌شود.

## اجرای سریع

```bash
# تست
go test ./internal/...          # 197 pass

# build
go build -o bin/khabavar ./cmd/khabavar

# اجرای محلی (نیاز به PostgreSQL)
KH_ROLE=api KH_DB_URL=postgres:///khabavar KH_TOKEN_SIGNING_KEY=dev \
  ./bin/khabavar
```

## push به GitHub

مخزن مقصد `d3l4var/khabavar` است و شاخهٔ کاری `main`:

```bash
git push origin main
```
