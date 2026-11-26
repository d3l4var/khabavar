# KHABAVAR — خواب‌آور

**اکوسیستم تجارت و محتوای تخصصی خواب** · ایران · fa-IR · RTL
وب‌سایت اصلی + فروشگاه + مجله + هویت مرکزی + پنل مدیریت

> سند مادر: `KH-MGMT-001` · تاریخ: ۲۰۲۶-۱۱-۲۶
> وضعیت: **بک‌اند P3–P7 سبز و قابل اجرا** · فرانت‌اند: scaffold · فهرست دقیق شکاف‌ها: [`docs/ACTIVITY-LOG.md`](docs/ACTIVITY-LOG.md) §۵
> مدل تجاری: فروشگاه تخصصی مدل A (PRD §۲)
> پشته: Go (تک‌باینری، سه unit) · React+TS+Vite (چهار اپ) · PostgreSQL 16 · REST/OpenAPI · Linux + systemd

این مخزن **اسکلت + پیاده‌سازی P3 Core، P4 Commerce، P5a catalog و P5b pricing** است: ساختار کامل ماژول‌ها، مهاجرت‌ها،
قرارداد API، خط لوله CI، واحدهای systemd و چهار فرانت‌اند، به‌علاوهٔ کد واقعی لایهٔ platform و ماژول‌های
identity/authorization/audit/worker/catalog/pricing/commerce. جریان **OIDC Authorization Code + PKCE** (`GET /oauth/authorize`،
`POST /oauth/token` با چرخش refresh token و id_token)، **dispatcher واقعی outbox** (`domain_events`
با retry/dead-letter)، **ماژول catalog** (درخت دسته‌بندی، محصول/Variant، **دروازهٔ انتشار MA-1/MA-6**،
endpoints فروشگاهی `GET /api/v1/categories`، `/products/{id}`، `/categories/{id}/products`)، **ماژول pricing**
(هزینه تمام‌شدهٔ ۸ جزئی، قیمت فروش با **دروازهٔ تأیید نقش مجاز**، اقتصاد واحد Variant با فرمول R-05 §۱۹،
هشدار BR-06 مشارکت منفی، endpoint `GET /api/v1/variants/{id}/price` که اطلاعات محرمانه را از مشتری پنهان
می‌کند — R-05 §۲۴) و **ماژول commerce** (ایجاد سفارش با تطابق مبالغ، snapshot آدرس، یک Intent فعال/سفارش،
تأیید پرداخت با رویداد OrderPaid، لغو سفارش، webhook idempotent، ردیابی سفارش مهمان) پیاده‌سازی شده‌اند و
**`go vet` + `go build` + ۲۳۰ آزمون سبز** هستند (بدون نیاز به دیتابیس؛ آزمون‌های PG با
`KH_TEST_PG_DSN`). ماژول‌های suppliers/inventory/fulfillment/media/content/notifications/
ecosystem نیز پیاده‌سازی و در unit `api` mount شده‌اند.

**کنترل دسترسی پنل:** همهٔ `/api/v1/admin/*` پشت `authorization.Authenticate` +
`RequireAudited` هستند؛ کد مجوز هر روت در `cmd/khabavar/routes.go` (`adminPermissions`)
اعلان می‌شود و روت ادمین بدون مجوز در startup panic می‌کند (IAM §۱). رد دسترسی در
`audit_logs` ثبت می‌شود (IAM §۷).

راهنمای اجرایی: [`IMPLEMENTATION_GUIDE.md`](IMPLEMENTATION_GUIDE.md) · قرارداد:
[`api/openapi.yaml`](api/openapi.yaml) · CI: [`.github/workflows/ci.yml`](.github/workflows/ci.yml)

---

## شروع سریع

```bash
go mod tidy
make migrate-up          # ۱۱ مهاجرت روی PG 16 + seed
make seed                # ۱۰ نقش، ۲۴ مجوز، ۴ کلاینت OIDC
make run-api             # KH_ROLE=api ./bin/khabavar
make run-identity        # KH_ROLE=identity
make run-worker          # KH_ROLE=worker
```

متغیرها فقط از env خوانده می‌شوند؛ نام‌ها در [`.env.example`](.env.example)، مقادیر هرگز در مخزن (DEP §۵).

## ساختار

```
cmd/khabavar/        یک باینری، KH_ROLE=identity|api|worker (ADR-DB-002)
internal/platform/   config · db · log · health · ratelimit · idempotency · testpg
internal/modules/    ۱۳ ماژول دامنه با مرز مصوب (SAD §۴.۱)
migrations/          ۱۲ مهاجرت up + ۱۲ down stub (forward-only) + embed.go
seed/                001_seed_baseline.sql — نقش‌ها، مجوزها، OIDC، موضوعات مجله
web/                 main · shop · magazine · admin (React+TS+Vite)
deploy/systemd/      سه unit (تک‌بیناری)
deploy/nginx/        چهار server block + rate limit auth
.github/workflows/   ci.yml — ۱۱ کار، هفت دروازه ادغام (R-CI v1.1)
api/                 openapi.yaml v1.1.0 + .spectral.yaml (دروازهٔ CI)
docs/                ایندکس مسیر مطالعه R-01..R-19 + ACTIVITY-LOG
```

## مرزهای غیرقابل مذاکره

1. **تک‌باینری**: سه unit از همان باینری با `KH_ROLE` اجرا می‌شوند — نه باینری جدا (ADR-DB-002).
2. **مالکیت داده**: هیچ ماژولی جدول ماژول دیگر را مستقیم UPDATE/INSERT نمی‌کند (SAD §۴.۲).
3. **Forward-only migrations**: down فقط اضطراری با `emergency_rollback_migration` (DEP §۶).
4. **Mock-first**: همه Adapterها (پرداخت/پیامک/حمل/Storage) Mock شروع می‌شوند — جایگزینی فقط پیکربندی (ADR-ARCH-011).
5. **Idempotency**: هر عملیات حساس مالی با `Idempotency-Key` + Audit (ADR-ARCH-008/010).
6. **RBAC از Seed**: کد نام permission را چک می‌کند، هرگز نقش را هاردکد نمی‌کند (IAM §۱).
7. **پول IRR int64**: تومان فقط لایه نمایش (ADR-DATA-001).

## اسناد مرجع (R-numbered)

مسیر مطالعه کامل در [`docs/INDEX.md`](docs/INDEX.md). مهم‌ترین‌ها برای پیاده‌سازی:
SAD v2.1 (R-16) · OpenAPI v1.1.0 (R-07) · DB Migrations v2.0 (R-14) · IAM Matrix v1.1 (R-11) ·
CI v1.1 (R-CI) · Deployment Spec v1.1 (R-18) · Test Strategy v1.1 (R-17) · Roadmap v1.0 (R-19).

## مجوز

اختصاصی — مالکیت پروژه خواب‌آور. انتشار عمومی ممنوع.
