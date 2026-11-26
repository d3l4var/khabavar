# CONTRIBUTING — KHABAVAR

این سند قواعد مشارکت در مخزن خواب‌آور را تعریف می‌کند. رعایت این قواعد برای ادغام در `main` الزامی است
و توسط دروازه‌های CI (`.github/workflows/ci.yml`) راستی‌آزمانی می‌شود.

## ۱. قواعد وابستگی ماژول (SAD §۴.۲ — غیرقابل مذاکره)

۱. هیچ ماژولی جدول ماژول دیگر را مستقیم UPDATE/INSERT نمی‌کند؛ مالکیت داده با ماژول است.
۲. ارتباط بین‌ماژولی فقط از طریق API داخلی (فراخوانی متد در مرز ماژول) + رویدادهای Outbox برای
   اثرات جانبی.
۳. وابستگی دایره‌ای ممنوع؛ جهت مجاز: `fulfillment → commerce → {catalog, inventory, pricing}` و
   `content → catalog` (فقط id).
۴. رویدادهای دامنه Commerce §۱۰ به‌صورت `domain_events` منتشر می‌شوند؛ مصرف‌کننده‌ها idempotent‌اند.
۵. هر عملیات حساس: `Idempotency-Key` + Audit (ADR-ARCH-008/010).

جدول کامل ماژول‌ها، موجودیت‌ها و وابستگی مجاز در `internal/modules/*/doc.go` و SAD v2.1 §۴.۱.

## ۲. کنوانسیون کد

- **Go**: `gofmt` + `go vet` + `golangci-lint` سبز (دروازه `lint`). نام بسته = نام ماژول.
- **مبلغ**: `int64` به IRR (ADR-DATA-001)؛ تومان فقط لایهٔ نمایش.
- **زمان**: `TIMESTAMPTZ`/UTC ذخیره، Asia/Tehran نمایش (ADR-DATA-002).
- **وضعیت‌ها**: `TEXT`+`CHECK` نه ENUM زودهنگام (ADR-DATA-004).
- **شناسه‌ها**: UUID + شناسهٔ تجاری جدا مثل `KHA-ORD-…` (ADR-DB-003).
- **مجوزها**: نام `module.entity.action`؛ کد نام permission را چک می‌کند، هرگز نقش را هاردکد نمی‌کند (IAM §۱).
- **Frontend**: RTL Native، اعداد فارسی، Mobile First، توکن از Design System (ممنوعیت کد رنگ مستقیم، ADR-FE-002).

## ۳. مهاجرت

- ترتیب: `identity → catalog → supplier → inventory → pricing → commerce → fulfillment → content → audit → db_roles → oidc_refresh_tokens` (DB v2 §۱۶).
- Forward-only: مهاجرت اجراشده تغییر نمی‌کند؛ down فقط اضطراری با `emergency_rollback_migration` (DEP §۶).
- seed جدا از migration؛ فقط دادهٔ سیستمی؛ هیچ دادهٔ فرضی عملیاتی.
- هر قید CHECK جدید باید یک آزمون نقض‌کننده داشته باشد (TS §۱).

## ۴. قرارداد API

- مرجع: `api/openapi.yaml` (v1.1.0). تغییر قرارداد قبل از پیاده‌سازی مشتری.
- `openapi-validate` با Spectral + قواعد سفارشی `api/.spectral.yaml`: Idempotency-Key روی POSTهای مالی،
  Money int64، تفکیک scope staff.
- PRISM در حالت mock بالا می‌آید و smoke قرارداد روی همهٔ مسیرها اجرا می‌شود (CI §۵).

## ۵. شاخه و انتشار

- شاخه‌ها: `main` (production)، `staging`، feature branch‌ها از `staging`.
- هر PR باید از هفت دروازه عبور کند (CI §۱). e2e/perf شبانه روی staging.
- انتشار staging روی push به `staging`، production روی push به `main` با محیط guarded + تأیید دستی (DEP §۱).
- هر ریلیز فقط یک علت تغییر؛ دو ریلیز هم‌زمان ممنوع؛ پنجرهٔ کم‌ترافیک ۲-۵ بامداد (DEP §۶).

## ۶. رازها

- رازها (DB URL، رمز توکن، کلیدهای درگاه/پیامک) هرگز در مخزن؛ فقط نام متغیر در `.env.example`.
- روی سرور در `/etc/khabavar/<unit>.env` با مالک root و دسترسی ۶۰۰ (DEP §۳).
- چرخش رمز DB و توکن هر ۹۰ روز (DEP §۵).
