# khabavar — Project Manifest

**نسخه:** v1.0 (P1–P10 کامل)
**تاریخ:** 2026-11-26
**مخزن:** https://github.com/d3l4var/khabavar
**شاخه:** github-sync (۱۷ کامیت آماده push)

## خلاصه

khabavar یک پلتفرم تجارت الکترونیک تخصصی سلامت خواب است — فروشگاه محصول + مجله علمی + احراز هویت OIDC. بک‌اند Go با معماری ماژولار (SAD §۴.۲)، پایگاه‌داده PostgreSQL، و چهار برنامه وب React+TS+Vite.

## آمار نهایی

| معیار | مقدار |
|-------|-------|
| ماژول‌های Go | ۱۳ (catalog, pricing, commerce, suppliers, inventory, fulfillment, media, content, ecosystem, notifications, identity, authorization, worker) + audit, shared |
| پکیج‌های پلتفرم | ۸ (config, db, health, idempotency, log, ratelimit, security, testpg) |
| migrationهای DB | ۱۱ |
| تست‌های سبز | ۱۹۷ (۰ FAIL) |
| endpointهای HTTP | ~۶۰ |
| سندهای مشخصات | ۱۴ (R-01..R-CI) |
| سندهای منبع | ۱۱ (۰۱-۱۱) |
| ران‌بوک‌ها | ۴ (Deployment, Backup/Restore, Incident Response, Monitoring) |
| فازهای تکمیل‌شده | P1–P10 (همه) |

## ساختار پروژه

```
khabavar/
├── cmd/khabavar/          # نقطه ورود (سه unit: identity/api/worker)
├── internal/
│   ├── modules/           # ۱۳ ماژول دامنه + audit + shared
│   └── platform/          # config, db, health, idempotency, log, ratelimit, security
├── migrations/            # ۱۱ migration SQL (identity→oidc_refresh_tokens)
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
| P2 | Brand & Design System | ✅ سند طراحی (R-04) |
| P3 | Core Platform | ✅ foundation، DB، OIDC، IAM، audit، امنیت |
| P4 | Magazine Platform | ✅ ماژول content (گردش کار تحریریه، دروازه انتشار) |
| P5 | Store Platform | ✅ catalog، pricing، inventory، media، suppliers، جستجو |
| P6 | Commerce Operations | ✅ commerce، fulfillment، refund، تطبیق تراکنش |
| P7 | Ecosystem Integration | ✅ جستجوی یکپارچه، ناوبری مشترک |
| P8 | Administration | ✅ notifications module (SMS/Email adapters) |
| P9 | Security & Performance | ✅ security headers، CORS، HSTS، CSP |
| P10 | Production Launch | ✅ runbooks، deploy manifests، backup/restore، monitoring |

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

۱۷ کامیت آماده push روی شاخه `github-sync`:

```bash
GH_TOKEN=ghp_xxx bash scripts/push_to_github.sh
```
