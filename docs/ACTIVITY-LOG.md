# KHABAVAR — سجل فعالیت‌های توسعه

> این سند رکورد کارهایی است که روی مخزن انجام شده: چه چیزی پیدا شد، چه چیزی اصلاح شد،
> با چه شاهدی تأیید شد، و چه چیزی باقی مانده. هر ورودی باید قابل بازتولید باشد.
>
> مخزن: <https://github.com/d3l4var/khabavar> · شاخه: `main`
> دورهٔ ثبت: ۲۰۲۶-۱۱-۲۶

---

## ۱. نقطهٔ شروع

پروژه به‌صورت آرشیو تحویل داده شد، نه مخزن:

```
/home/dlp/Projects/KHABAVAR/
├── khabavar-complete.tar.gz            # کد + اسناد
├── khabavar-complete.zip
├── khabavar-full-with-binary.tar.gz
└── khabavar-linux-amd64                # باینری ساخته‌شده
```

مخزن مقصد از قبل وجود داشت (`d3l4var/khabavar`، عمومی، شاخهٔ `main`، HEAD `30fe01a`)
با ۹۰ فایل و ۱۰ کامیت `P3: ...`.

### کاری که انجام شد

1. استخراج آرشیو به `/home/dlp/Projects/KHABAVAR/khabavar`.
2. حذف `download/khabavar-repo/` — کپی کهنه و تکراری از `internal/modules` بود که
   به‌عنوان پکیج جداگانه در `go test ./...` کامپایل می‌شد (۲۴ فایل).
3. ادغام `.gitignore`: نسخهٔ آرشیو قواعد `web/*/dist`, `vendor/`, `*.env` را از دست
   داده بود؛ نسخهٔ remote حفظ و `skills/` و `__pycache__/` به آن اضافه شد.
4. `git init` + `fetch origin` + `reset origin/main` تا تاریخچهٔ موجود حفظ شود (نه
   orphan commit).
5. ۱۶ کامیت منطقی — یک ماژول در هر کامیت، به سبک خودِ مخزن، با ارجاع به بند سند.
6. push به `main` و تأیید سمت GitHub (۱۹۹ فایل، HEAD مطابق).

---

## ۲. وضعیت اولیهٔ کد (اندازه‌گیری‌شده، نه ادعا)

| معیار | مقدار |
|-------|-------|
| فایل‌های مخزن | ۱۹۹ |
| Go (prod / test) | ۱۰٫۷۹۸ / ۶٫۹۲۷ خط |
| ماژول دامنه | ۱۳ + `audit` + `shared` |
| مهاجرت | ۱۱ (بعداً ۱۲) |
| تست | ۲۱۲ تابع، همه سبز |
| `go build` / `go vet` | سبز |
| `gofmt -l .` | **۳۱ فایل قرمز** |
| پوشش تست | ۰٪ تا ۱۰۰٪، میانگین خیلی زیر آستانهٔ R-CI |

`gofmt` در کامیت `7652e1b` اصلاح شد (فقط تراز، بدون تغییر رفتار).

---

## ۳. شکاف‌هایی که در مطالعه پیدا شد

| # | شکاف | شدت | وضعیت |
|---|------|-----|-------|
| ۱ | همهٔ `/api/v1/admin/*` بدون احراز هویت mount شده بودند؛ `authorization.Require` وجود داشت ولی هیچ‌جا صدا زده نمی‌شد | بحرانی (امنیتی) | **اصلاح شد** |
| ۲ | باینری `KH_ROLE=api` اصلاً بالا نمی‌آمد: تداخل الگوی `ServeMux` | بحرانی (اجرا) | **اصلاح شد** |
| ۳ | outbox با `FOR UPDATE SKIP LOCKED` در autocommit رویداد را دوبار dispatch می‌کرد | بحرانی (داده) | **اصلاح شد** |
| ۴ | `refunds` با اسکیمای خیالی (`currency`, `processed_at`) کوئری می‌شد | بحرانی (اجرا) | **اصلاح شد** |
| ۵ | `ConfirmPaymentForOrder` هیچ `payment_attempts` نمی‌نوشت ⇒ هر بازپرداخت واقعی شکست می‌خورد | بحرانی (تجاری) | **اصلاح شد** |
| ۶ | reconciliation فرمول اشتباه داشت (هرگز true نمی‌شد) | بالا | **اصلاح شد** |
| ۷ | خطای SQL خام به کلاینت برمی‌گشت | بالا (امنیتی) | **اصلاح شد** |
| ۸ | تست‌های PG یکدیگر را می‌شکستند (یک دیتابیس مشترک + drop schema) | بالا (CI) | **اصلاح شد** |
| ۹ | ماژول `notifications` به worker وصل نبود؛ `ecosystem` mount نشده بود | متوسط | **اصلاح شد** |
| ۱۰ | `.github/workflows/ci.yml` وجود نداشت (INDEX به آن لینک می‌داد) | بالا | **اصلاح شد** |
| ۱۱ | `api/openapi.yaml` وجود نداشت (INDEX به آن لینک می‌داد) | متوسط | **اصلاح شد** |
| ۱۲ | `IMPLEMENTATION_GUIDE.md` وجود نداشت (README به آن ارجاع می‌داد) | متوسط | **اصلاح شد** |
| ۱۳ | روت‌های ادمین کاتالوگ/قیمت/audit/کاربران طبق قرارداد پیاده نشده‌اند | بالا | **باقی** |
| ۱۴ | فرانت‌اند چهار اپ فقط ۱۰ خط scaffold است | بالا | **باقی** |
| ۱۵ | پوشش تست زیر آستانهٔ R-CI (۹۰٪ مالی / ۷۵٪ سایر) | متوسط | **باقی** |
| ۱۶ | `PROJECT_MANIFEST.md` «P1–P10 کامل» می‌گفت | متوسط (صداقت گزارش) | **اصلاح شد** |

---

## ۴. اصلاحات، با شاهد

### ۴.۱ RBAC پنل مدیریت — کامیت `0a14983`

**قبل:** هر روت `/api/v1/admin/*` بدون توکن ۲۰۰ می‌داد.

**بعد:**

- `internal/modules/authorization/db.go` — `DBChecker` روی
  `admin_users ⋈ admin_user_roles ⋈ role_permissions ⋈ permissions`،
  با `status='ACTIVE'`، شناسهٔ بدشکل → `(false, nil)` (نه ۵۰۰).
- `internal/modules/authorization/middleware.go` — `Authenticate` (Bearer →
  `identity_sessions` → ctx؛ توکن مشتری → ۴۰۱) و `RequireAudited` (رد → ۴۰۳ + audit).
- `cmd/khabavar/routes.go` — جدول اعلانی `adminPermissions` (۳۳ روت) و `apiMux.admin`
  که روت بدون مجوز را در startup panic می‌کند.
- `internal/modules/shared/actor.go` — `ActorID(ctx)` تا ماژول‌ها بدانند «چه‌کسی» بدون
  import کردن `authorization` (SAD §۴.۲).

**شاهد (باینری واقعی + PG واقعی):**

```
healthz / readyz                     200 / 200
GET  /api/v1/categories              200
POST /admin/suppliers  بدون توکن     401
POST /admin/suppliers  توکن مشتری    401
POST /admin/suppliers  STORE_MANAGER 403   (supplier.manage ندارد)
GET  /admin/orders/{id}/refunds  FINANCE 403   (order.view ندارد)
POST /admin/orders/{id}/refunds  STORE_MANAGER 403   (refund انحصاری FINANCE)
POST /admin/orders/{id}/refunds  FINANCE 400   (مجوز داشت، بدنه ناقص)
```

### ۴.۲ باینری api بالا نمی‌آمد — کامیت `d2f5b6e`

`GET /orders/by-number/{number}` با `GET /orders/{id}/fulfillment` برای مسیر
`/orders/by-number/fulfillment` هر دو match می‌شد و `ServeMux` در ثبت panic می‌کرد:

```
panic: pattern "GET /api/v1/orders/by-number/{number}" ... conflicts with pattern ...
```

**اصلاح:** `/orders/{id}` حالا هم UUID و هم شمارهٔ تجاری (`KHA-ORD-YYYY-NNNNNN`) را
می‌فهمد — همان شکلی که قرارداد افشا می‌کند. جدول روت به `mountRoutes()` منتقل شد و
`TestMountRoutesHasNoPatternConflicts` کل جدول را روی `ServeMux` تازه mount می‌کند،
پس این کلاس خطا در build شکست می‌خورد نه در deploy.

### ۴.۳ outbox رویداد را دوبار dispatch می‌کرد — کامیت `0a14983`

تست هم‌روندی: `45 duplicate claims`. علت: قفل ردیف‌های `FOR UPDATE SKIP LOCKED` در
autocommit به‌محض پایان statement آزاد می‌شد.

**اصلاح:** مهاجرت `000012_outbox_claim` (`claimed_at`, `claimed_by`, ایندکس جزئی) و
`Claim` به‌صورت یک `UPDATE … RETURNING` اتمیک با lease ۶۰ ثانیه. `MarkFailed` lease را
آزاد می‌کند (retry فوری)، `MarkPublished` پاکش می‌کند.

**شاهد:** تست هم‌روندی سبز؛ worker واقعی ۵ رویداد را drain کرد و همه `published_at` گرفتند.

### ۴.۴ بازپرداخت با اسکیمای واقعی هم‌تراز شد — کامیت `d2f5b6e`

`refunds` ستون `currency` ندارد، `completed_at` دارد نه `processed_at`، و
`payment_attempt_id` اجباری است. همچنین `REFUNDED` در `chk_orders_status` نیست.

**اصلاح:** `Refund` مطابق جدول؛ statusها `REQUESTED…COMPLETED`؛ ارجاع اجباری به
attempt موفق؛ `approved_by` از actor احراز هویت‌شده؛ refund کامل → `CLOSED`؛ ثبت
audit `order.refund.issued`؛ reconciliation به `paid_total == Σ attempts` اصلاح شد.

### ۴.۵ پرداخت، attempt ثبت نمی‌کرد — کامیت `ffbe420`

بدون ردیف `payment_attempts`، `refunds.payment_attempt_id` چیزی برای ارجاع نداشت و هر
بازپرداخت واقعی با `order has no succeeded payment attempt to refund` شکست می‌خورد.

**اصلاح:** `PaymentAttempt` + `PaymentAttemptStore` + `CreateAttempt` با
`ON CONFLICT (idempotency_key) DO UPDATE … RETURNING` و کلید مشتق‌شده از intent
(`confirm:<intent-id>`)، تا webhook تکراری attempt دوم نسازد.

**شاهد — E2E کامل روی HTTP + PostgreSQL:**

```
1. POST /orders                                   → id
2. POST /orders/{id}/payment-intents               → intent id
3. POST /orders/{id}/confirm-payment               → {"status":"paid"}   (65000 = 60000+5000)
4. تکرار همان درخواست                              → {"status":"already_paid"}
5. select count(*) from payment_attempts           → 1 | SUCCEEDED
6. POST /admin/orders/{id}/refunds (FINANCE)       → refund id
7. GET  /admin/orders/{id}/refunds                 → PaymentAttemptID ✔ ApprovedBy ✔ COMPLETED
8. GET  /orders/{id}                               → Status=PAID PaidTotal=65000 RefundedTotal=20000
9. GET  /admin/orders/{id}/reconcile               → Reconciled=true NetPaid=45000
10. audit_logs                                     → order.refund.issued | 20000
11. domain_events                                  → RefundIssued
12. KH_ROLE=worker                                 → "worker: drained events=5"، همه published
```

### ۴.۶ ایزولاسیون تست PG — کامیت `0a14983`

`go test ./internal/...` با `KH_TEST_PG_DSN` شکست می‌خورد چون همهٔ بسته‌ها موازی روی
یک دیتابیس اجرا می‌شدند و cleanup هر تست `DROP SCHEMA public CASCADE` می‌زد.

**اصلاح:** `testpg.Setup` برای هر تست یک schema (`test_<random>`) می‌سازد و
`search_path` استخر را روی آن می‌گذارد؛ `ApplyAll` یک advisory lock می‌گیرد.
همچنین `SeedCustomer` predicate ایندکس جزئی را تکرار می‌کند
(`ON CONFLICT (phone) WHERE phone IS NOT NULL`) و شناسه‌های غیر-UUID در تست‌های
OIDC/outbox اصلاح شدند.

### ۴.۷ نشت خطا — کامیت `d2f5b6e`

`writeError` متن خام درایور را برمی‌گرداند:

```json
{"error":"ERROR: column \"currency\" does not exist (SQLSTATE 42703)", ...}
```

حالا ۵xx فقط `{"error":"internal error","code":"internal_error"}` می‌دهد و جزئیات در
لاگ می‌رود؛ ۴xx کد ماشین‌خوان پایدار می‌گیرد.

### ۴.۸ CI، قرارداد و راهنما — کامیت `docs/ci`

- `.github/workflows/ci.yml` — ۱۱ کار، ۷ دروازهٔ ادغام (lint، openapi-validate،
  db-migrate، unit، integration، security، a11y) + build + e2e/perf شبانه + deploy
  با environment guarded. دروازهٔ db-migrate خودش ماتریس RBAC را assert می‌کند
  (۱۰ نقش، ۲۴ مجوز، `order.refund.approve` فقط FINANCE).
- `api/openapi.yaml` — قرارداد مصوب v1.1.0 (کپی R-07) + `api/.spectral.yaml`.
  `spectral lint` → **۰ error**، ۲۹ warning.
- `IMPLEMENTATION_GUIDE.md` — راهنمای گام‌به‌گام (setup، تست، افزودن ماژول، RBAC،
  outbox، استقرار).

### ۴.۹ روت‌های خواندنی پنل طبق قرارداد — کامیت `feat(admin)`

پنج روت قراردادی که پیاده نشده بودند و فقط خواندن‌اند، اضافه شدند:

| روت | مجوز | منبع |
|-----|------|------|
| `GET /api/v1/admin/orders` | `order.view` | `commerce.OrderListStore` |
| `GET /api/v1/admin/audit-logs` | `audit.read` | `audit.ListStore` (فیلتر `action`/`actor_id`) |
| `GET /api/v1/admin/users` | `identity.user.manage` | `identity.AdminStore` (با نقش‌ها) |
| `GET /api/v1/admin/roles` | `identity.user.manage` | `identity.AdminStore` (با مجوزها) |
| `GET /api/v1/admin/permissions` | `identity.user.manage` | `identity.AdminStore` |

**شاهد — ماتریس مجوز روی باینری واقعی:**

```
route                          STORE_MANAGER  FINANCE  SUPER_ADMIN
/admin/orders                        200         403       200
/admin/audit-logs                    403         200       200
/admin/users                         403         403       200
/admin/roles                         403         403       200
/admin/permissions                   403         403       200
```

`/admin/audit-logs` رکورد واقعی بازپرداخت را برگرداند
(`order.refund.issued`، actor = شناسهٔ FINANCE، old/new values کامل).

### ۴.۱۰ سطح نوشتنی کاتالوگ و قیمت‌گذاری (P5b-admin) — کامیت `feat(admin-write)`

دوازده روت نوشتنی که قرارداد اعلام کرده بود و سرویس‌شان از قبل وجود داشت، به HTTP و
RBAC وصل شدند:

| روت | مجوز |
|-----|------|
| `POST /admin/products` | `catalog.product.edit` |
| `GET /admin/products/{id}` | `catalog.product.view` |
| `POST /admin/products/{id}/publish` | `catalog.product.publish` |
| `POST /admin/products/{id}/status` | `catalog.product.publish` |
| `POST /admin/categories` | `catalog.attribute.manage` |
| `POST /admin/variants` | `catalog.product.edit` |
| `POST /admin/variants/{id}/cost-records` | `pricing.cost.manage` |
| `GET /admin/variants/{id}/cost-records` | `pricing.cost.manage` |
| `POST /admin/variants/{id}/price-records` | `pricing.price.approve` |
| `GET /admin/variants/{id}/price-records` | `pricing.price.approve` |
| `POST /admin/price-records/{id}/approve` | `pricing.price.approve` |
| `GET /admin/variants/{id}/economics` | `reports.financial.view` |

تصمیم‌های ریزبینانه‌ای که در پیاده‌سازی گرفته شد:

- **تأییدکنندهٔ قیمت از نشست خوانده می‌شود، نه از بدنه.** `approved_by` =
  `shared.ActorID(ctx)` (R-05 §۹.۲). اگر actor خالی باشد، ۴۰۱ برمی‌گردد — fail closed.
- **`POST /admin/products/{id}/status` فقط `PAUSED` را می‌پذیرد.** انتشار روت و دروازهٔ
  جداگانه دارد، تا پنل نتواند با یک نوشتن وضعیت، MA-1/MA-6 را دور بزند.
- **دروازهٔ انتشار ۴۲۲ می‌دهد** (نه ۴۰۹ قدیمی) با کد `publish_gate_missing` و دلیل
  مشخص، به‌جای نقض خام CHECK.
- **`ErrNoValidCost` در مسیر نوشتن ۴۰۹ است، نه ۴۰۴:** نبودِ هزینهٔ معتبر یک تعارض وضعیت
  است که اپراتور باید حل کند.
- **سه دستهٔ حساس IAM §۷ حالا audit می‌شوند:** `catalog.product.published`،
  `catalog.product.status_changed`، `pricing.cost.recorded`، `pricing.price.recorded`،
  `pricing.price.approved` — با تزریق `SetAuditor` و بدون import کردن پکیج audit.
- **JSON همهٔ پاسخ‌ها snake_case شد.** `Product`, `Variant`, `Category`, `CostRecord`,
  `PriceRecord`, `UnitEconomics` تگ نداشتند و کلیدهای Go (`ID`, `SellingPrice`) برمی‌گرداندند
  — ناسازگار با بقیهٔ API و با قرارداد. یک تست قدیمی که کلید Go را assert می‌کرد به‌روز شد.

**شاهد — زنجیرهٔ کامل QC-05/R-05 روی باینری واقعی:**

```
POST /admin/products  (بدون فیلدهای دروازه)     201
POST /admin/products/{id}/publish                422  publish_gate_missing
POST /admin/products  (با sleep_relevance+spec)  201
POST /admin/products/{id}/publish                200  {"status":"PUBLISHED"}
POST /admin/variants  (SKU-MP-001)               201
POST /admin/variants  (همان SKU)                 409  duplicate_sku
POST /admin/variants/{id}/cost-records           201  total_cost=515000  currency=IRR
POST /admin/variants/{id}/price-records (450k)   201  warnings=[BR-06 …]
POST /admin/variants/{id}/price-records (900k)   201  PENDING_APPROVAL
POST /admin/price-records/{id}/approve (FINANCE) 200  approved_by=<FINANCE id>
POST /admin/price-records/{id}/approve (تکرار)   409  already_approved
GET  /admin/variants/{id}/economics              200  contribution=385000 rate=0.428
GET  /variants/{id}/price (storefront)           200  بدون فیلدهای محرمانه
GET  /admin/products/{id} (STORE_MANAGER)        200  محصول DRAFT هم دیده می‌شود
GET  /products/{id} (storefront، همان DRAFT)     404  منتشر نشده
```

ماتریس مجوز روی همین روت‌ها:

```
POST /admin/products                    STORE_MANAGER=400 (مجوز دارد)  FINANCE=403
POST /admin/variants/{id}/cost-records  STORE_MANAGER=400               FINANCE=400
POST /admin/price-records/{id}/approve  STORE_MANAGER=404               FINANCE=404
GET  /admin/variants/{id}/economics     STORE_MANAGER=403               FINANCE=200
```

(۴۰۰ = مجوز داشت، بدنه ناقص؛ ۴۰۴ = مجوز داشت، رکورد نیست. این‌ها رفتار درست‌اند.)

### ۴.۱۱ ویرایش محصول و بستن بن‌بست انتشار — کامیت `feat(catalog-update)`

هنگام اجرای E2E کاتالوگ یک **بن‌بست واقعی** پیدا شد: محصولی که بدون فیلدهای دروازه
ساخته می‌شد هرگز قابل انتشار نبود، چون هیچ راهی برای تکمیل `sleep_relevance` /
`spec_source` / `spec_last_reviewed_at` بعد از ساخت وجود نداشت. سرویس `UpdateProduct`
هم نبود.

**اصلاح:** `PATCH /api/v1/admin/products/{id}` با مجوز `catalog.product.edit`.

- **PATCH واقعی است، نه overwrite:** بدنه با pointer خوانده می‌شود تا «فیلد نیامده» با
  «فیلد خالی‌شده» قاطی نشود.
- **وضعیت از این مسیر تغییر نمی‌کند:** انتشار و توقف روت و دروازهٔ خودشان را دارند، پس
  PATCH نمی‌تواند MA-1/MA-6 را دور بزند.
- **محصول PUBLISHED نمی‌تواند فیلد دروازه را از دست بدهد** → ۴۲۲، وگرنه یا CHECK نقض
  می‌شد یا محصول زنده بی‌صدا از فروشگاه می‌افتاد.
- **دسته‌بندی‌ها فقط وقتی جایگزین می‌شوند که `category_ids` در بدنه باشد** (حتی آرایهٔ
  خالی = پاک کردن). جایگزینی در همان تراکنش انجام می‌شود.
- `catalog.product.updated` هم audit می‌شود.

**شاهد — همان محصول گیرکرده، روی باینری واقعی:**

```
select status …                        DRAFT | sleep_relevance = -
POST /admin/products/{id}/publish      422  publish_gate_missing
PATCH /admin/products/{id}             200  sleep_relevance=SLEEP_RELATED، عنوان تغییر کرد
POST /admin/products/{id}/publish      200  {"status":"PUBLISHED"}
GET  /products/{id} (storefront)       200  محصول حالا در فروشگاه دیده می‌شود
PATCH {"sleep_relevance":""}           422  (محصول منتشرشده دروازه را از دست نمی‌دهد)
PATCH {"slug":""}                      400
PATCH با توکن FINANCE                  403  (catalog.product.edit ندارد)
audit_logs                             catalog.product.updated | catalog.product.published
```

تست‌های جدید (`update_test.go`، ۶ مورد): بن‌بست انتشار، slug خالی، شکستن دروازهٔ محصول
منتشرشده، شناسهٔ ناموجود، جایگزینی صریح دسته‌بندی‌ها، و نبودِ update store.

### ۴.۱۲ پنل تأمین: موجودی، حرکت، رزرو — کامیت `feat(inventory-admin)`

سه روت قراردادی `admin-supply` اضافه شد:

| روت | مجوز | توضیح |
|-----|------|-------|
| `GET /admin/stock` | `inventory.manage` | فیلتر `warehouse_id` و `q` (SKU/عنوان)، `available = on_hand − reserved` |
| `POST /admin/stock/movements` | `inventory.adjust` | اصلاح موجودی فقط با ثبت حرکت (DB §8.3) |
| `GET /admin/reservations` | `inventory.manage` | فیلتر `status` و `expires_before`، همراه `order_number` |

**تصمیم ایمنی (مستند):** روت حرکت فقط انواع اثرگذار بر `on_hand` را می‌پذیرد
(`RECEIPT`, `RETURN`, `ADJUSTMENT`, `DAMAGE`, `TRANSFER`). انواع `RESERVATION`،
`RELEASE` و `SALE` روی `reserved_quantity` اثر می‌گذارند و متعلق به جریان سفارش‌اند؛
اجازه دادنشان از پنل، ناوردای `available = on_hand − reserved` را خراب می‌کرد (DB §8.2).
این سه نوع با ۴۰۰ و پیام روشن رد می‌شوند.

**دو باگ که همین E2E پیدا کرد:**

1. کوئری موجودی از `w.code` استفاده می‌کرد؛ ستون واقعی `warehouse_code` است → هر
   `GET /admin/stock` با ۵۰۰ می‌افتاد.
2. موجودی منفی ۵۰۰ برمی‌گرداند، چون `ErrNegativeOnHand` در نگاشت خطا نبود؛ حالا ۴۰۹
   با کد `insufficient_stock` است.

همچنین صفحهٔ خالی حالا `items: []` می‌دهد نه `null` (قرارداد آرایه اعلام کرده).

**شاهد:**

```
POST /admin/stock/movements RECEIPT +10 (SUPER_ADMIN)   201
POST /admin/stock/movements DAMAGE  -3                  201
POST /admin/stock/movements ADJUSTMENT -999             409  insufficient_stock
POST /admin/stock/movements RESERVATION +1              400  movement type not allowed
POST /admin/stock/movements با STORE_MANAGER            403  (inventory.adjust ندارد)
GET  /admin/stock?q=SKU-MP                              200  on_hand=7 available=7
GET  /admin/reservations                                200  items=[] page.total=0
GET  /admin/reservations?expires_before=not-a-date       400
stock_movements                                         2 ردیف (رد شدن‌ها چیزی ننوشت)
```

### ۴.۱۳ بازبینی روند توسعه — دو ایراد در ابزار خودمان

پیش از ادامه، کل مسیر از ابتدا بازبینی شد (build/vet/gofmt، ۲۳۶ تست، کلاستر PG تازه،
اعمال ۱۳ مهاجرت + seed، ماتریس RBAC). دو ایراد در **ابزار تأیید** پیدا شد، نه در محصول:

1. **assertion دروازهٔ `db-migrate` غلط بود.** نوشته بودم `order.refund.approve` نباید به
   هیچ نقشی جز FINANCE داده شود — اما SUPER_ADMIN طبق IAM §۴ هر ۲۴ مجوز را دارد. یعنی
   دروازه روی یک seed **درست** قرمز می‌شد. اصلاح شد (`NOT IN ('FINANCE','SUPER_ADMIN')`)
   و یک assert دوم اضافه شد که SUPER_ADMIN دقیقاً ۲۴ مجوز داشته باشد.
2. **ماتریس RBAC هیچ تستی نداشت** — فقط با psql دستی چک شده بود. حالا `seed/embed.go` +
   `seed/seed_test.go` با ۵ تست: شمارش‌ها (۱۰ نقش/۲۴ مجوز/۴ کلاینت/۱۱ موضوع)،
   SUPER_ADMIN = ۲۴، انحصار بازپرداخت، هشت نقش با must/must-not، و جداسازی scope در OIDC.
   کار `integration` در CI هم از `./internal/...` به `./...` تغییر کرد تا این گیت اجرا شود.

### ۴.۱۴ نشت محصول منتشرنشده در فروشگاه — کامیت `fix(catalog-visibility)`

اولین تست انتگرالِ `DBStore` کاتالوگ (`catalog_pg_test.go`) یک **نشت واقعی** پیدا کرد:
`GetProductDetail` هیچ فیلتری روی وضعیت ندارد — که برای پنل درست است — ولی همان متد
پشت روت عمومی `GET /api/v1/products/{id}` هم بود. یعنی هر کسی با داشتن UUID می‌توانست
محصول `DRAFT`/`PAUSED`/`DISCONTINUED` را بخواند (R-04 §۵، QC-05).

**اصلاح:** متد جداگانهٔ `GetStorefrontProduct` که فقط `PUBLISHED` را برمی‌گرداند و
در غیر این صورت `ErrProductNotFound` می‌دهد؛ روت عمومی از آن استفاده می‌کند و روت پنل
همان `GetProductDetail` می‌ماند.

**شاهد (باینری واقعی):**

```
POST /admin/products (DRAFT)            201
GET  /products/{id}  (فروشگاه)          404
GET  /admin/products/{id} (پنل)         200
GET  /products/{id}  (محصول PUBLISHED)  200
```

> تصحیح سجل: در §۴.۱۰ برای یک محصول DRAFT پاسخ ۴۰۴ فروشگاه ثبت شده بود، اما آن یک
> مشاهدهٔ موردی بود نه یک قاعدهٔ تضمین‌شده — کد فیلتری نداشت. حالا قاعده صریح، تست‌شده
> (`TestStorefrontDetailHidesUnpublished` + تست PG) و روی باینری تأیید شده است.

همچنین اولین تست انتگرال کاتالوگ اضافه شد (`catalog_pg_test.go`): چرخهٔ کامل دسته →
محصول → انتشار → Variant → جستجو → توقف، به‌علاوهٔ اثبات اینکه قید
`chk_products_publish_gates` در سطح دیتابیس هم انتشار بدون دروازه را رد می‌کند.

### ۴.۱۵ تست انتگرال DBStore برای چهار ماژول — کامیت `test(pg-store)`

پوشش تست واحد با fake store اندازه‌گیری می‌شد، پس کد `DBStore` (جایی که هر دو باگ
اسکیمایی رخ داد) عملاً تست نشده بود. چهار فایل تست انتگرال اضافه شد:

| ماژول | تست‌ها | چه چیزی را قفل می‌کند |
|-------|--------|------------------------|
| pricing | ۵ | هشت جزء هزینه، دروازهٔ تأیید قیمت، پنجرهٔ اعتبار، BR-06، اقتصاد واحد |
| suppliers | ۳ | ماشین حالت ۸ وضعیتی، یکتایی (تأمین‌کننده، محصول)، تاریخچهٔ قیمت، ارزیابی وزنی |
| inventory | ۴ | ناوردای `available = on_hand − reserved`، oversell، convert، منبع رزرو، sweep انقضا |
| fulfillment | ۴ | ماشین حالت، قاعدهٔ منبع، ارسال چندبسته‌ای، تایم‌لاین، سقف تخصیص |

**دو نقص واقعی که همین تست‌ها پیدا کردند:**

1. **تاریخچهٔ قیمت تأمین‌کننده ناقص بود.** `CreateSupplierVariant` قیمت اولیه را در
   `supplier_variant_price_history` ثبت نمی‌کرد، پس بازسازی تایم‌لاین قیمت از تاریخچه
   غیرممکن بود (§7.3: هر قیمت باید قابل ردیابی باشد). حالا در همان تراکنش ثبت می‌شود.
2. **خطای ثبت رویداد ارسال بی‌صدا بلعیده می‌شد.** `TransitionShipmentStatus` مقدار
   `_, _ =` داشت؛ اگر `source` مجاز نبود (CHECK فقط `MANUAL`/`CARRIER_API`/`WEBHOOK`)،
   وضعیت مرسوله عوض می‌شد ولی رویدادی در تایم‌لاین نمی‌ماند. حالا `source` **قبل** از
   تغییر وضعیت اعتبارسنجی می‌شود و خطای ثبت رویداد برگردانده می‌شود.

**پوشش (با PG، قبل → بعد):**

```
pricing      45.5% → 61.7%      catalog     26.8% → 62.0%
suppliers    23.1% → 60.1%      inventory   23.3% → 44.3%
fulfillment  24.1% → 52.2%
```

گام coverage در CI از کار `unit` به کار `integration` منتقل شد — چون تست‌های DBStore
همان‌جا اجرا می‌شوند و اندازه‌گیری بدون دیتابیس عدد fake-store می‌داد. آستانه‌ها
(۹۰٪ مالی / ۷۵٪ سایر) دست‌نخورده‌اند و گام هنوز `continue-on-error` است.

### ۴.۱۶ تست انتگرال commerce و content — کامیت `test(pg-store-2)`

دو ماژول باقی‌ماندهٔ بدون تست `DBStore` پوشش داده شدند (۸ تست جدید):

| ماژول | چه چیزی قفل شد |
|-------|-----------------|
| commerce | ساخت سفارش با محاسبهٔ مبالغ (۲×۵۰۰۰۰+۱۰۰۰۰=۱۱۰۰۰۰)، یکتایی شمارهٔ تجاری، یافتن با UUID و با شماره، قواعد لغو، رویدادهای outbox، فهرست پنل با فیلتر وضعیت، قید `chk_orders_totals` |
| content | گردش کار تحریریه، **دروازهٔ انتشار** (بدون منبع → رد و وضعیت تغییر نمی‌کند)، رابطهٔ مقاله-محصول با دلیل اجباری، زنجیرهٔ ترجمه، انتشار زمان‌بندی‌شده |

**یک نقص واقعی:** قید MA-5 در دیتابیس نبود. ستون `relation_reason TEXT NOT NULL` بود،
یعنی رشتهٔ خالی را می‌پذیرفت؛ قاعده فقط در سرویس اعمال می‌شد و یک نوشتن مستقیم می‌توانست
«دلیل ارتباط» را به یک فیلد تشریفاتی تبدیل کند. مهاجرت `000013_relation_reason_check`
اضافه شد با `CHECK (btrim(relation_reason) <> '')` — همان الگوی
`chk_products_publish_gates`.

**پوشش (با PG):** commerce ۴۴٪ → ۶۶.۹٪ · content ۲۱.۵٪ → ۴۴.۴٪

### ۴.۱۷ شروع فرانت‌اند: Design System + اپ `main` — کامیت `feat(web-main)`

تا اینجا هر چهار اپ ۱۰ خط scaffold بودند. اکنون:

**`web/shared/` — Design System مشترک (P2)**
- `tokens.css`: پالت **مصوب** برند (Midnight `#10172B`، Deep Navy، Sleep Blue، Lavender،
  Moon White، Mist، Charcoal، Success/Warning/Error) به‌صورت CSS custom property،
  به‌علاوهٔ مقیاس فاصله/شعاع/سایه/تایپوگرافی. کامپوننت‌ها فقط از توکن استفاده می‌کنند —
  قاعدهٔ سند برند («استفادهٔ مستقیم از کد رنگ ممنوع»).
- `components.tsx`: `Container`, `Button`, `Badge`, `Card`, `Grid`, `Header`, `Footer`,
  `ProductCard`, `ArticleCard`, `StateBlock` (سه حالت مشترک Loading/Empty/Error) و
  `useAsync` که همان سه حالت را یک‌جا مدیریت می‌کند.
- `api.ts`: کلاینت تایپ‌شدهٔ API با پاکت خطای یکسان، به‌علاوهٔ `formatToman` (تبدیل ریال→تومان
  فقط در لایهٔ نمایش، ADR-DATA-001).

**`web/main` — سایت اصلی**
- روتر hash بدون هیچ وابستگی جدید (لیست deps عمداً کوچک است).
- صفحه‌ها: خانه (مقالات + محصولات)، فروشگاه با جست‌وجو، جزئیات محصول، مجله، جزئیات مقاله،
  جست‌وجوی یکپارچه (product+article از ماژول ecosystem).
- RTL بومی (`dir="rtl"`, `lang="fa-IR"`, ویژگی‌های منطقی CSS).

**زیرساخت:** `web/package.json` با npm workspaces (`shared`, `main`, `shop`, `magazine`,
`admin`) تا `web/shared` بتواند `react` را از store مشترک resolve کند — بدون آن، tsc و
Vite فایل‌های بیرون از ریشهٔ اپ را نمی‌توانستند type-check/باندل کنند.

**دو ایراد در scaffold که اصلاح شد:**
1. `tsconfig.json` گزینهٔ نامعتبر `"rtl": true` داشت (tsc چنین گزینه‌ای ندارد) → حذف شد.
2. اسکریپت build `tsc -b` بود که فایل `.js` را **کنار سورس** منتشر می‌کرد
   (`src/main.js`). به `tsc --noEmit && vite build` تغییر کرد و `*.tsbuildinfo` به
   `.gitignore` رفت.

**شاهد:**

```
npm run build (web/main)      tsc --noEmit + vite build ✓
                              dist/assets/index-*.js 156.69 kB │ gzip 49.89 kB
dev server                    /  → 200، <html lang="fa-IR" dir="rtl">
                              /src/main.tsx → 200
                              /@fs/.../shared/components.tsx → 200
                              /@fs/.../shared/tokens.css → 200
پروکسی API                    /api/v1/navigation → 200 (payload واقعی ecosystem)
                              /api/v1/articles   → 200
```

> این محدودیت در §۴.۱۸ رفع شد: Lightpanda نصب و اپ در مرورگر واقعی تأیید شد.

### ۴.۱۸ تأیید فرانت‌اند در مرورگر واقعی + دو باگ — کامیت `fix(api): empty lists`

**Lightpanda نصب شد** (`~/.local/bin/lightpanda`, v1.1.0-nightly) و اپ `main` در یک
مرورگر واقعی اجرا و بازرسی شد. سه چیز روشن شد:

1. **Lightpanda اسکریپت `type="module"` را اجرا نمی‌کند** — اپ با باندل ESM اصلاً رندر
   نمی‌شد. برای تأیید، یک باندل IIFE با esbuild ساختم (`dist-lp/`، فقط برای بازرسی،
   در `.gitignore`) و همان اپ ۱۰۰٪ رندر شد. **این محدودیت مرورگر است، نه باگ محصول:**
   باندل تولیدی همان ESM استاندارد می‌ماند که مرورگرهای واقعی اجرا می‌کنند.

2. **`"data": null` به‌جای `[]`** — همهٔ endpointهای فهرستی وقتی نتیجه خالی بود `null`
   برمی‌گرداندند، چون slice تهی در Go به JSON `null` تبدیل می‌شود. قرارداد آرایه اعلام
   کرده و کلاینت هم روی `.length` می‌شکست (خطای واقعی: `Cannot read properties of null
   (reading 'length')`). **۲۳ نقطه در ۸ فایل `*_db.go`** اصلاح شد و کلاینت هم
   defensively نوشته شد.

3. **JSON چند ساختار Go-style بود** — `StorefrontPrice`، `OrderInput`، `OrderItemInput`
   تگ نداشتند. snake_case شدند تا قرارداد ساخت سفارش از سمت کلاینت قابل استفاده باشد.

**شاهد — اجرای واقعی در Lightpanda:**

```
errs: []                        root_len: 5132
h1:  خواب بهتر، از آگاهی شروع می‌شود
h2:  تازه‌ترین مقالات | محصولات        cards: 3
nav: #/ · #/shop · #/magazine · #/search + حساب من/مدیریت (خارجی، از ecosystem)
/articles خالی →  «هنوز مقاله‌ای منتشر نشده است.»  (حالت Empty درست)
#/shop        → فروشگاه، ۳ کارت، input جست‌وجو
#/magazine    → مجله
#/search      → جست‌وجو
#/product/…   → بالش طبی مموری + «کد کالا: SKU-MP-001» (دادهٔ واقعی API)
#/nope        → صفحهٔ «پیدا نشد»
```

### ۴.۱۹ اپ `shop` و مسیر خرید کامل در مرورگر — کامیت `feat(web-shop)`

**`web/shop`** ساخته شد: فهرست محصولات با جست‌وجو، صفحهٔ محصول با **قیمت زندهٔ مصوب**
(`GET /variants/{id}/price`)، سبد سمت کلاینت (localStorage — بک‌اند cart ندارد)، تسویه و
صفحهٔ تأیید سفارش.

**قاعدهٔ مهم در تسویه:** قیمت هر قلم **دوباره** از API خوانده می‌شود، نه از سبد؛ یک تب
کهنه نمی‌تواند قیمت قدیمی را ثبت کند.

**سه باگ واقعی که همین مسیر در مرورگر پیدا کرد:**

1. **شمارهٔ سفارش اجباری بود، در حالی که مالک آن سرور است.** `POST /orders` بدون
   `order_number` با خطای `commerce: order_number is required` رد می‌شد — یعنی کلاینت وب
   مجبور بود شمارهٔ ترتیبی سرور را از خودش بسازد. مهاجرت `000014_order_number_seq`
   (سکانس)، متد `NextOrderNumber` در `Store`، و تولید خودکار در `CreateOrder` **قبل از**
   اعتبارسنجی. تست واحد قدیمی که «رد شدن» را انتظار داشت با تست «تولید شدن» جایگزین شد.
2. **`Order`/`OrderItem` تگ JSON نداشتند** — پاسخ `/orders/{id}` کلیدهای Go
   (`OrderNumber`, `GrandTotal`) برمی‌گرداند و صفحهٔ تأیید «ناعدد تومان» نشان می‌داد.
   snake_case شدند.
3. **`GET /articles` خالی `null` می‌داد** (در §۴.۱۸ اصلاح شد).

**شاهد — مسیر خرید کامل در Lightpanda (مرورگر واقعی، بدون خطا):**

```
product   → بالش طبی مموری | ۹۰٬۰۰۰ تومان
add       → #/cart | ۱ قلم در سبد | localStorage نوشته شد
checkout  → فرم پر شد (۰۹۱۲۰۰۰۰۰۹۹ / تهران / تهران)
submit    → #/order/77ce29a0-…
تأیید     → شمارهٔ سفارش: KHA-ORD-2026-000002 | وضعیت: PENDING_PAYMENT | مبلغ: ۹۰٬۰۰۰ تومان
errs: []
```

### ۴.۲۰ اپ `admin` + روت فهرست محصولات — کامیت `feat(web-admin)`

**شکاف بک‌اند:** قرارداد `GET /api/v1/admin/products` را داشت ولی پیاده نشده بود، پس پنل
هیچ راهی برای دیدن پیش‌نویس‌ها نداشت (جست‌وجوی عمومی فقط `PUBLISHED` می‌دهد). افزوده شد:
`catalog.AdminListStore.List` با فیلتر وضعیت/عبارت و ستون محاسبی
`publish_gate_satisfied`، روت با مجوز `catalog.product.view` (شمار روت‌های ادمین: ۵۸).

**`web/admin`** ساخته شد: دروازهٔ توکن، سفارش‌ها (با فیلتر وضعیت)، محصولات (تکمیل دروازهٔ
انتشار + انتشار)، انبار (موجودی + ثبت حرکت)، Audit، نقش‌ها.

**اصل طراحی:** پنل **هیچ‌گاه خودش مجوز تعیین نمی‌کند** — هر روت در API با
`authorization.Require` و ماتریس seed بررسی می‌شود و ۴۰۳ عیناً به کاربر نشان داده
می‌شود. این‌طور منبع حقیقت یکی می‌ماند (IAM §۱) و ماتریس در کلاینت تکرار نمی‌شود.

**شاهد — پنل در Lightpanda، با توکن SUPER_ADMIN:**

```
بدون توکن  → /admin/products 401، همهٔ پنج روت با توکن 200
ورود       → «سفارش‌ها» | ۴ سفارش واقعی (KHA-ORD-2026-000001/000002 · ۹۰٬۰۰۰ تومان)
محصولات    → ۴ کارت؛ پیش‌نویس: DRAFT + «دروازهٔ انتشار ناقص» + دکمهٔ تکمیل دروازه
انتشار     → «محصول منتشر شد.» → کارت به PUBLISHED تغییر کرد
errs: []
```

**باگ هارنس (نه محصول):** پروکسی تست فقط `GET/POST` را فوروارد می‌کرد و `PATCH` را با
**501** رد می‌کرد؛ پس دکمهٔ «تکمیل دروازه» در مرورگر کار نمی‌کرد. `do_PATCH/PUT/DELETE`
اضافه شد. (تأیید با `curl` مستقیم: PATCH → 200.)

### ۴.۲۱ ورود واقعی پنل: ایمیل + گذرواژه — کامیت `feat(identity): staff login`

**شکاف:** `admin_users.password_hash` از مهاجرت اول وجود داشت ولی استفاده نمی‌شد
(«نسخه اول: ورود پنل با OTP؛ فیلد برای آینده»)، و `OTPVerify` فقط نشست **مشتری** صادر
می‌کرد — یعنی هیچ راهی برای ورود کارمند به پنل نبود؛ پنل مجبور بود توکن دستی بگیرد.

**افزوده شد:**
- `Service.AdminLogin` با **bcrypt (cost 12)** — و برای ایمیل ناشناخته هم یک مقایسهٔ
  bcrypt روی هش ساختگی اجرا می‌شود تا زمان پاسخ وجود حساب را لو ندهد (بدون enumeration).
- `Service.IssueAdminSession` + `Session.AdminUserID` + درج `admin_user_id` در
  `identity_sessions` (نشست کارمند هرگز `customer_id` ندارد).
- `POST /api/v1/auth/admin/login` روی واحد identity.
- `scripts/dev/002_dev_admin.sql` — **بیرون از `seed/`** تا baseline جاسازی‌شده هرگز
  گذرواژه حمل نکند؛ فقط برای محیط توسعه.

**شاهد — API:**

```
root@khabavar.ir + گذرواژهٔ درست → 200 {access_token}
گذرواژهٔ نادرست                  → 401
ایمیل ناشناخته                   → 401   (همان پاسخ، بدون تفکیک)
```

**شاهد — پنل در Lightpanda:**

```
فرم ورود  → email,password
ورود      → «سفارش‌ها» | ۴ سفارش | توکن در sessionStorage
errs: []
```

**باقی‌مانده:** تبادل کد OIDC در مرورگر (Authorization Code + PKCE سمت کلاینت). سرور
`/oauth/authorize` و `/oauth/token` را دارد و تست‌شده است؛ کلاینت وب فعلاً مستقیم وارد
می‌شود.

### ۴.۲۲ اپ `magazine` + فیلتر موضوع — کامیت `feat(web-magazine)`

**دو شکاف قبل از فرانت:**
1. `Article`/`Topic`/`ArticleSource` تگ JSON نداشتند — `/articles` کلیدهای Go
   (`Title`, `Slug`, `PublishedAt`) برمی‌گرداند. snake_case شدند.
2. `GET /api/v1/articles` فیلتر موضوع نداشت، پس صفحهٔ موضوع مجبور بود در کلاینت
   فیلتر کند (گمراه‌کننده). پارامتر `?topic=<id>` اضافه شد و به
   `ListArticlesByTopic` (که وجود داشت ولی روت نداشت) وصل شد.

**`web/magazine`** ساخته شد: خانه (موضوع‌ها + تازه‌ترین مقالات)، صفحهٔ مقاله
(`#/article/{slug}`)، صفحهٔ موضوع (`#/topic/{id}`)، فهرست موضوع‌ها.

**نکتهٔ امنیتی:** بدنهٔ مقاله به‌صورت **متن ساده** و پاراگراف‌به‌پاراگراف رندر می‌شود،
نه HTML — ویرایشگر نمی‌تواند مارک‌آپ به صفحهٔ عمومی تزریق کند.

**شاهد — زنجیرهٔ کامل محتوا (API):**

```
POST /admin/articles (بدون منبع) → publish 409  ← دروازهٔ ADR-CONTENT-001
POST /admin/articles/{id}/sources (GUIDELINE) → 201
submit 200 → approve 200 → publish 200
GET /articles                → مقاله با کلیدهای snake_case
GET /articles?topic=<id>     → همان مقاله در موضوع «علوم خواب»
GET /articles/{slug}         → 200
```

**شاهد — مجله در Lightpanda:**

```
خانه      → «مجلهٔ خواب» | ۶ چیپ موضوع | ۱ کارت مقاله | «۵ آذر ۱۴۰۵»
مقاله     → h1 + «۵ آذر ۱۴۰۵ · ORIGINAL» + خلاصه + بدنه (۳ پاراگراف)
موضوع     → «علوم خواب» | ۱ کارت
موضوع‌ها  → ۱۱ موضوع
errs: []
```

### ۴.۲۳ صفحهٔ محصول در پنل: ویرایش، گزینه، زنجیرهٔ قیمت — کامیت `feat(admin-product)`

**شکاف بک‌اند:** `GET /admin/products/{id}` وضعیت دروازهٔ انتشار را برنمی‌گرداند، پس پنل
مجبور بود قاعدهٔ MA-1/MA-6 را در کلاینت بازسازی کند. `ProductDetail.PublishGateSatisfied`
اضافه شد و سرور همان قاعده‌ای را که `PublishProduct` اعمال می‌کند گزارش می‌دهد.

**`web/admin` — صفحهٔ `#/product/{id}`:** ویرایش فیلدها (PATCH)، فهرست گزینه‌ها، افزودن
گزینه، ثبت بهای تمام‌شده، ثبت قیمت، تأیید قیمت، انتشار.

**باگ واقعی در کلاینت:** جدول `price_records` ستون `status` **ندارد** — تأیید با
`approved_by` بیان می‌شود. پنل همهٔ رکوردها را «در انتظار تأیید» نشان می‌داد و دکمهٔ
تأیید را برای رکورد تأییدشده هم می‌آورد. حالا از `approved_by` مشتق می‌شود.

**شاهد — زنجیرهٔ کامل روی محصول پیش‌نویس:**

```
افزودن گزینه از پنل   → SKU-DRAFT-01 (DRAFT)
ثبت بهای تمام‌شده     → 201
ثبت قیمت ۹۰۰٬۰۰۰      → PENDING_APPROVAL
تأیید قیمت            → 200
قیمت فروشگاه          → {"selling_price":900000,"currency":"IRR"}
پنل                   → «۹۰٬۰۰۰ تومان APPROVED» و «۹۰٬۰۰۰ تومان PENDING_APPROVAL» + یک دکمهٔ تأیید
نشان دروازه           → PUBLISHED | دروازهٔ انتشار کامل (دکمهٔ انتشار پنهان)
errs: []
```

### ۴.۲۴ ایجاد محصول + دسته‌بندی + یک باگ نوشتن خاموش — کامیت `fix(catalog): category write`

**شکاف seed:** جدول `categories` **خالی** بود — ۱۱ موضوع مجله seed شده بود ولی هیچ
دستهٔ محصولی نه. یعنی محصول قابل دسته‌بندی نبود و فروشگاه ناوبری نداشت. درخت دو سطحی
(۴ دستهٔ اصلی + ۶ زیردسته) با `ON CONFLICT (slug) DO NOTHING` به seed اضافه شد.

**`web/admin`:** فرم «محصول جدید» (عنوان/اسلاگ خودکار/خلاصه) که محصول DRAFT می‌سازد و
به صفحهٔ جزئیات می‌رود، و انتخابگر دسته‌بندی (درخت عمومی، انتخاب فعلی از خود محصول
بارگذاری می‌شود تا اپراتور دسته‌های نادیده را پاک نکند).

**باگ نوشتن خاموش (جدی):** `Service.UpdateProduct` مجموعهٔ دسته‌ها را **بعد** از فراخوانی
store به `p.CategoryIDs` نسبت می‌داد، ولی store همان `p` را می‌گیرد و از رویش
`DELETE`+`INSERT` می‌کند. نتیجه: لینک‌ها پاک می‌شدند، هیچ‌چیز درج نمی‌شد، API **۲۰۰**
برمی‌گرداند و پاسخ مجموعهٔ جدید را echo می‌کرد — یعنی پاسخ دروغ می‌گفت.

```
قبل از رفع:  PATCH {"category_ids":[X]} → 200 ، پاسخ category_ids:[X] ، DB rows = 0
بعد از رفع:  PATCH {"category_ids":[X]} → 200 ، DB rows = 1
```

تست رگرسیون `TestUpdateProduct_PersistsCategoryReplacement` اضافه شد (روی store حافظه‌ای
همان نشت را می‌گیرد).

**شاهد — چرخهٔ کامل از پنل در Lightpanda:**

```
محصول جدید      → «پتوی وزن‌دار» → #/product/38d5651c-… (DRAFT، دروازهٔ ناقص)
دسته‌بندی‌ها      → ۱۰ چیپ با تورفتگی درخت (تشک و تخت / — تشک بادی / …)
ذخیرهٔ دسته‌بندی  → ۲۰۰ و ردیف در product_categories
errs: []
```

### ۴.۲۵ بازبینی روند + دو یافتهٔ تازه — کامیت `fix(silent-writes)`

**بازبینی (همه با خروجی واقعی):**

```
HEAD == origin/main == 90781af · tree پاک · ۵۱ کامیت
go build / go vet / gofmt -l  → پاک
go test ./...                → ۱۶ بسته ok (با و بدون PG)
build هر چهار اپ            → main, shop, admin, magazine
بازتولید از صفر (کلاستر تازه، پورت 55434):
  ۱۴ مهاجرت بدون خطا · seed بدون خطا · ۵۳ جدول
  roles=10 perms=24 oauth_clients=4 topics=11 categories=10
```

**یافتهٔ ۱ — نصب تازه هیچ کاربر کارمندی ندارد.** baseline seed عمداً حساب کارمند
نمی‌سازد (گذرواژه نباید در مخزن باشد)، ولی نتیجه این بود که `admin_users` روی دیتابیس
تازه **صفر** است و پنل اصلاً قابل ورود نیست؛ `scripts/dev/002_dev_admin.sql` هم فقط
`UPDATE` می‌کرد روی ردیفی که وجود نداشت (بی‌اثر). حالا همان فایل سه حساب توسعه را
`INSERT` می‌کند (root/ops/fin با نقش‌های SUPER_ADMIN/STORE_MANAGER/FINANCE) و idempotent
است. شاهد روی دیتابیس تازه: ورود root و ops → **200**.

**یافتهٔ ۲ — همان کلاس «نوشتن خاموش» در دو مسیر دیگر.** سه `UPDATE` بدون بررسی
`RowsAffected` روی ردیف ناموجود «موفق» گزارش می‌شدند:

- `UpdateOrderStatus` و `UpdateOrderPaid` (پول!) → حالا `ErrOrderNotFound`
- `UpdateProductStatus` (انتشار/توقف) → حالا `ErrProductNotFound`

تست‌های PG: `TestPG_StatusUpdatesRejectUnknownOrder` و
`TestPG_StatusUpdateRejectsUnknownProduct` — هر دو PASS.

**بازرسی الگو:** تنها یک مسیر با پارامتر `replace… bool` وجود داشت (همان باگ دسته‌بندی،
رفع‌شده). ۱۰ `UPDATE` دیگر بدون بررسی باقی مانده که همه داخل تراکنش با
`SELECT … FOR UPDATE` قبلی هستند (انبار، OAuth) — ریسک پایین، در فهرست کارها ثبت شد.

### ۴.۲۶ ورود مرورگری OIDC + دو باگ مسدودکنندهٔ ورود — کامیت `feat(oidc-login)`

**شکاف:** `/oauth/authorize` بدون نشست به `GET /api/v1/auth/otp/login?continuation=…`
رجوع می‌داد که **mount نشده بود** — یعنی هیچ کلاینت ثالثی نمی‌توانست جریان را در مرورگر
تمام کند (dead-end واقعی).

**افزوده شد:** صفحهٔ ورود سمت‌سرور (HTML ساده، بدون فریم‌ورک) + `POST` که کد را
می‌سنجد، نشست می‌سازد، `continuation` را با HMAC بازمی‌خواند و جریان را ادامه می‌دهد.
توکن نشست هرگز در URL نمی‌رود.

**دو باگ مسدودکننده که همین مسیر بیرون کشید:**

1. **`OTPVerify` مقدار پیش‌فرض purpose را اعمال نمی‌کرد.** `OTPStart` خالی را `LOGIN`
   می‌گیرد ولی verify نه؛ کلاینتی که در فراخوانی دوم `purpose` را نمی‌فرستاد، چالش را با
   `purpose=''` می‌جست، پیدا نمی‌کرد و پاسخ **«invalid code»** می‌گرفت — که مثل کد اشتباه
   به نظر می‌رسد، نه مثل فیلد جاافتاده.
2. **ورود مشتری روی schema واقعی کاملاً شکسته بود.** `UpsertCustomerByPhone` از
   `ON CONFLICT (phone)` استفاده می‌کرد، ولی ایندکس یکتا **جزئی** است
   (`uq_customers_phone … WHERE phone IS NOT NULL`). Postgres بدون تکرار همان شرط، ایندکس
   را نمی‌شناسد → `42P10` → هر ورود با OTP در آخرین قدم ۵۰۰ می‌داد. تست‌های واحد با
   store حافظه‌ای این را نمی‌دیدند.

**شاهد — جریان کامل با curl:**

```
GET /oauth/authorize (بدون نشست) → 302 به صفحهٔ ورود (قبلاً 404)
GET  /api/v1/auth/otp/login       → 200 (HTML)
POST /api/v1/auth/otp/login       → {"redirect_url":"…?code=932b225e…&state=xyz"}
POST /oauth/token (code+PKCE)     → {"access_token":"df328d27…","token_type":"Bearer","expires_in":900,"refresh_token":"8cd82907…"}
```

تست‌های PG جدید: `TestPG_UpsertCustomerByPhoneIsIdempotent`،
`TestOTPVerify_DefaultsPurposeToLogin` — هر دو PASS.

### ۴.۲۷ بازرسی پوشش PG و بستن سه شکاف پرمخاطره — کامیت `test(pg-coverage)`

**درس دو باگ قبلی:** هر متد store که SQL اجرا می‌کند باید حداقل یک بار روی schema واقعی
اجرا شود؛ تست با store حافظه‌ای `42P10` و نوشتن خاموش را نمی‌بیند.

**ابزار تکرارپذیر:** `scripts/audit-pg-coverage.py` — فهرست متدهای SQL-دار بدون تست PG را
می‌دهد و با `--fail-under` می‌تواند در CI گیت شود.

```
قبل:  SQL methods=82  covered=48 (58.5%)  uncovered=34
بعد:  SQL methods=82  covered=60 (73.2%)  uncovered=22
```

**تست‌های تازه (همه PASS روی schema واقعی):**

- `TestPG_PaymentIntentAndAttemptReads` — intent فعال، `SuccessfulAttemptID`،
  `SumSucceededAttempts`، `UpdateOrderRefunded`
- `TestPG_ReservationAndBalanceWrites` — upsert موجودی (دو بار: باید update کند نه
  duplicate)، رزرو، خواندن، تغییر وضعیت
- `TestPG_ShipmentWrites` — محموله، تغییر وضعیت، رخداد، جمع تخصیص

**باگ چهارم از همین کلاس در حین تست:** `UpdatePaymentIntentStatus` هم `RowsAffected` را
بررسی نمی‌کرد → حالا `ErrPaymentIntentNotFound`.

**۲۲ متد باقی‌مانده** (همه خواندنی یا مسیرهای کم‌ریسک: `GetCart`, `GetTopic`,
`GetWarehouse`, `media.*`, `suppliers.ListSupplierVariantsByVariant`, …) در فهرست
`scripts/audit-pg-coverage.py` ثبت شده‌اند.

### ۴.۲۸ بستن پوشش PG تا ۱۰۰٪ و سخت‌کردن گیت — کامیت `test(pg-coverage-100)`

```
قبل:  81 متد SQL-دار · 60 پوشش (73.2%) · 21 بدون تست
بعد:  81 متد SQL-دار · 81 پوشش (100%)  ·  0
```

**تست‌های تازه:** `media` (اولین تست PG این ماژول)، انبار (`GetWarehouse`,
`GetWarehouseByCode`)، کاتالوگ (`ListProductsByCategory` — با اثبات اینکه پیش‌نویس در
فهرست منتشرشده نمی‌آید)، محتوا (`GetTopic`, `UpdateTopicStatus`, `SetArticleEditor`,
`UpdateArticleStatus`, `DeleteArticleSource`, `AddProductArticleRelation` با اثبات الزام
`relation_reason` در سطح DB)، کامرس (`GetCart`, `UpdateCartStatus`, `ListOrderItems`)،
تکمیل سفارش (`GetFulfillmentItem`, `UpdateFulfillmentItemStatus`,
`UpdateFulfillmentOrderStatus`, `ListFulfillmentOrdersByOrder`,
`ListShipmentItemsByShipment`)، هویت (`FindAdminByEmail` — شامل اثبات اینکه حساب بدون
`password_hash` هرگز برنمی‌گردد)، تأمین‌کننده (`ListSupplierVariantsByVariant`).

**ابزار:** اسکریپت بازرسی حالا helperهای غیرصادراتی را نادیده می‌گیرد (از طریق فراخوان
صادراتی پوشش داده می‌شوند) و **گیت CI از advisory به blocking** تغییر کرد
(`--fail-under 95`, `continue-on-error: false`).

**شاهد:** `python3 scripts/audit-pg-coverage.py --fail-under 95` → `GATE_PASS`

### ۴.۲۹ دروازهٔ a11y با axe در مرورگر واقعی — کامیت `test(a11y)`

`@axe-core/cli` در این محیط قابل نصب نیست: درایور chromedriver را از میزبان Google
می‌گیرد که **از این موقعیت جغرافیایی مسدود است** («not available in your location»)، و
Chrome هم نصب نیست. اما Lightpanda CDP را می‌فهمد و axe-core در آن اجرا می‌شود.

`scripts/a11y-audit.mjs` — بدون هیچ وابستگی (WebSocket داخلی Node 26):
اپ بیلدشده را سرو می‌کند، axe را با تگ `<script>` تزریق می‌کند (payload ۶۵۰KB از کانال
`Runtime.evaluate` رد نمی‌شود)، مجموعهٔ WCAG 2 A/AA را اجرا و در صورت تخلف با کد ۱ خارج
می‌شود.

**شاهد در هر دو جهت:**

```
main, shop, admin, magazine  → PASS (۱۵ قاعده) · «a11y: clean»
صفحهٔ عمداً معیوب            → FAIL: critical button-name، critical image-alt
```

**محدودیت ثبت‌شده:** مرحلهٔ نصب Lightpanda در GitHub runner هنوز تأیید نشده، پس job در CI
`continue-on-error: true` است تا یک بار سبز شود و بعد blocking شود.

### ۴.۳۰ تلاش برای OIDC در پنل و نتیجه‌ای که باید ثبت شود — بدون کامیت

پنل را به جریان PKCE وصل کردم (`adminLoginViaOIDC`: نشست → `authorize` → کد →
`/oauth/token`). **کار نکرد و دلیلش یک واقعیت معماری است، نه باگ:**

- `fetch(..., {redirect:'manual'})` هدر `Location` را در اختیار صفحه نمی‌گذارد (پاسخ
  opaque است) — پس کد از پاسخ خوانده نمی‌شود.
- راه استاندارد، ناوبری کامل صفحه به `/oauth/authorize` است؛ ولی آن درخواست نمی‌تواند
  هدر `Authorization` بفرستد.

یعنی **`/oauth/authorize` برای کلاینت مرورگری به نشست کوکی نیاز دارد** (یا باید
`continuation` را بدون Bearer بپذیرد). این یک تصمیم طراحی است و باید با مالک گرفته شود؛
تا آن زمان پنل روی ورود مستقیم کارمند (ایمیل + گذرواژه) می‌ماند که تست‌شده است.

کد آزمایشی برگردانده شد (بدون dead code) و ورود پنل دوباره تأیید شد: «سفارش‌ها» با ۴ سفارش.

### ۴.۳۱ نشست کوکی برای authorize + گارد CSRF — کامیت `feat(oidc-cookie)`

تصمیم مالک: `/oauth/authorize` با کوکی HttpOnly. پیاده شد:

- `SetSessionCookie` — HttpOnly، `SameSite=Lax`، `Secure` وقتی TLS، و **`Path=/oauth`** تا
  کوکی هرگز به `/api/v1/admin/*` نرود (کوکی لو رفته روی API کارمند قابل replay نیست).
- `bearerFromCookie` فقط برای **GET** مقدار برمی‌گرداند؛ هر متد دیگر (POST) نادیده گرفته
  می‌شود.
- گارد CSRF: authorize با کوکی **فقط** وقتی پذیرفته می‌شود که `code_challenge` و `state`
  هر دو حاضر باشند — کد بدون verifier بی‌ارزش است و `state` پاسخ را به درخواست خود کلاینت
  می‌بندد.
- ورود کارمند (`/api/v1/auth/admin/login`) همان نشست را در کوکی هم می‌گذارد.

**باگ حین کار:** `writeOIDCErr` برای `ErrOIDCInvalidRequest` به شاخهٔ `default` می‌افتاد و
**۵۰۰** می‌داد؛ هر درخواست بدشکل authorize (نبود `code_challenge`، `response_type` غلط)
۵۰۰ می‌گرفت نه ۴۰۰. اصلاح شد.

**شاهد (curl، با/بدون کوکی):**

```
cookie + PKCE + state → 302 (کد صادر شد)
cookie، بدون state    → 400   ← گارد CSRF
cookie، بدون PKCE     → 400
بدون کوکی            → 302 به صفحهٔ ورود
```

**باقی‌مانده:** اتصال پنل به این جریان (ناوبری کامل صفحه → `/auth/callback` → تبادل کد).
یک بار پیاده شد ولی در هارنس تست به حلقهٔ ریدایرکت خورد و برگردانده شد؛ پنل فعلاً روی
ورود مستقیم کارمند است. `scripts/dev/003_dev_oidc_redirect.sql` برای وایت‌لیست کردن
`http://127.0.0.1:5180/auth/callback` در محیط توسعه اضافه شد.

### ۴.۳۲ Storybook برای Design System — کامیت `feat(storybook)`

`web/.storybook` با فریم‌ورک `@storybook/react-vite` ساخته شد و **همهٔ استوری‌ها در
`fa-IR` / RTL** رندر می‌شوند — کامپوننتی که در LTR سالم به نظر می‌رسد ممکن است در محصول
واقعی بشکند.

استوری‌ها: Button (اصلی/ثانویه/غیرفعال/پیوند)، Badge (چهار tone)، Card، StateBlock
(loading/empty/error)، ProductCard، ArticleCard، و صفحه‌آرایی (Header/Footer/Container).

**دروازهٔ a11y حالا روی Storybook هم اجرا می‌شود:**

```
/iframe.html?id=design-system-components--buttons        PASS (۱۰ قاعده)
/iframe.html?id=design-system-components--states         PASS
/iframe.html?id=design-system-components--product-cards  PASS
```

اسکریپت‌های npm: `storybook` (dev)، `build-storybook`، `a11y`.

### ۴.۳۳ اتصال پنل به جریان کوکی — تلاش دوم، باز هم برگردانده شد

پیاده شد (ناوبری کامل صفحه → `/auth/callback` → تبادل کد با verifier). در مرورگر
**daemon قفل می‌کند** هر بار که اپ با `?code=` بوت می‌شود — یعنی نشانهٔ یک حلقه در مسیر
بوت کلاینت است، نه فقط هارنس. چون نتوانستم تأیید کنم، دوباره برگرداندم و پنل روی ورود
مستقیم کارمند ماند (کار و تست‌شده).

**آنچه تأیید شده:** سمت سرور کامل کار می‌کند (۴.۳۱): کوکی ست می‌شود، authorize با کوکی +
PKCE + state کد می‌دهد، و بدون state/PKCE با ۴۰۰ رد می‌شود.

**گام بعدی مشخص:** پروب روی `document.location.search` در اولین خط بوت (قبل از React)،
تا معلوم شود آیا `?code=` اصلاً به اپ می‌رسد یا هارنس آن را در fallback از دست می‌دهد.

**تلاش سوم و یافته‌های آن (همه در هارنس، نه محصول):**

1. fallback هارنس با کوئری سالم است: `GET /auth/callback?code=abc` → **۲۰۰** و همان HTML
   بدون ریدایرکت (با curl تأیید شد).
2. `history.replaceState` از مسیر بوت حذف شد (مرورگر مینیمال ممکن است روی آن قفل کند) —
   مشکل باقی ماند.
3. **باگ واقعی هارنس:** `do_POST` فقط `/api/` را پروکسی می‌کرد، پس `POST /oauth/token` با
   **۴۰۵** رد می‌شد و کلاینت پیام «fetch error» می‌گرفت. حالا `/oauth/` هم پروکسی می‌شود.
4. پس از رفع آن، daemon مرورگر در هر بوت با `?code=` قفل می‌کند — حتی با session تازه.

**نتیجه:** سمت سرور تأییدشده؛ مسیر بوت کلاینت هنوز تأیید نشده و کد برگردانده شد.

### ۴.۳۴ ریشه پیدا شد: پروکسی تست خودش ریدایرکت را دنبال می‌کرد — کامیت `feat(admin-oidc)`

با صفحهٔ پروب ایستا معلوم شد مرورگر با کوئری مشکلی ندارد (`/probe-cb.html?code=abc` →
`search=?code=abc&state=xyz`) و اپ هم در `/auth/callback?x=1` سالم بوت می‌شود. پس مشکل
نه مرورگر بود و نه اپ.

**ریشه:** هارنس تست با `urllib.request.urlopen` پروکسی می‌کرد و urllib **خودش ۳۰۲ را دنبال
می‌کند**. یعنی مرورگر هرگز ۳۰۲ را نمی‌دید، خودش ریدایرکت را انجام نمی‌داد و کد هرگز به
`/auth/callback` نمی‌رسید. با یک `HTTPRedirectHandler` که `redirect_request` را `None`
برمی‌گرداند، پروکسی حالا ۳۰۲ و هدر `Location` را به مرورگر پاس می‌دهد.

**شاهد — ورود کامل پنل با OIDC در مرورگر:**

```
href: http://127.0.0.1:5180/auth/callback?code=8de3aacd…
h1:   سفارش‌ها
orders: 4   (KHA-ORD-2026-000002 · PENDING_PAYMENT · ۹۰٬۰۰۰ تومان)
```

پنل حالا واقعاً از جریان Authorization Code + PKCE رد می‌شود: ورود کارمند → کوکی HttpOnly
→ authorize → کد → تبادل با verifier → access token → API.

**نکتهٔ باقی‌مانده:** کد در نوار آدرس می‌ماند (عمداً، چون `history.replaceState` در مرورگر
مینیمال قفل می‌کرد). کد یک‌بارمصرف است، ولی رفرش همان تب خطای «جریان منقضی» می‌دهد و کاربر
باید دوباره وارد شود.

### ۴.۳۵ سه شکاف تولیدی که همین جریان OIDC لو داد — کامیت `fix(deploy-oidc)`

جریان OIDC که در ۴.۳۴ کار کرد، سه چیز را در **تنظیمات تولید** لو داد:

1. **`/auth/callback` در nginx به سرویس هویت پروکسی می‌شد.** بلوک `location /auth/` وجود
   داشت و مسیر callback پنل را می‌بلعید → در تولید ۴۰۴، در حالی که در تست کار می‌کرد.
   حالا `/auth/callback` از SPA سرو می‌شود و فقط `/api/v1/auth/` به سرویس هویت می‌رود.
2. **`/oauth/` اصلاً پروکسی نبود.** `/oauth/authorize` به `try_files` می‌افتاد و HTML
   برمی‌گرداند — یعنی ریدایرکت هرگز اتفاق نمی‌افتاد. برای هر چهار دامنه اضافه شد.
3. **کوکی در تولید `Secure` نبود.** سرویس Go پشت nginx، `r.TLS` را nil می‌بیند؛ کوکی
   HttpOnly بدون `Secure` روی HTTP هم فرستاده می‌شد. `requestIsSecure` حالا
   `X-Forwarded-Proto` را هم می‌خواند و nginx آن هدر (و `Host`/`X-Real-IP`/
   `X-Forwarded-For`) را می‌فرستد.

تست‌های تازه: `TestSessionCookieFlags` (سه حالت: TLS مستقیم، پشت nginx، HTTP ساده) و
`TestBearerFromCookieIgnoresNonGET` (کوکی هرگز متد تغییردهنده را احراز نمی‌کند).

### ۴.۳۶ `operationId` برای هر ۵۸ عملیات — کامیت `docs(openapi)`

قرارداد هیچ `operationId` نداشت (۰ از ۵۸)، پس تولید کلاینت از روی spec نام‌گذاری تصادفی
می‌گرفت. برای همهٔ عملیات نام camelCase اضافه شد (`postAuthOtpRequest`,
`getOauthAuthorize`, `getAdminOrders`, …).

**شاهد:** `npx @stoplight/spectral-cli lint api/openapi.yaml --ruleset api/.spectral.yaml
--fail-severity=error` → **0 errors** (۲۹ warning از نوع `operation-description` که از قبل
بودند و گیت CI روی error است، نه warning).

### ۴.۳۷ بازرسی ریزبینانهٔ لایهٔ استقرار و قرارداد — دو ابزار گارد

**۱) سطح env — ناسازگاری واقعی پیدا شد.** `scripts/audit-env.py` (کد `config.Load` را با
`.env.example` مقایسه می‌کند):

```
قبل:  خوانده‌شده ۱۹ · مستند ۱۶
  مستندنشده: KH_AUTH_CODE_TTL_SECONDS، KH_OIDC_ISSUER،
             KH_WORKER_INTERVAL_SECONDS، KH_WORKER_BATCH_SIZE
  مرده:      KH_CARRIER_ADAPTER   ← هیچ کدی آن را نمی‌خواند
بعد:  ۱۹/۱۹ — «env surface is consistent»
```

`KH_CARRIER_ADAPTER` حذف شد (با توضیح در فایل) چون آداپتر حامل هنوز وجود ندارد؛ نگه‌داشتنش
اپراتور را گمراه می‌کرد. سه متغیر worker هم مستند شدند (فقط برای `KH_ROLE=worker`).

**۲) مسیرها در برابر قرارداد — شکاف بزرگ R-07 با عدد.**

```
scripts/audit-routes.py
  mounted: 90   declared: 58
  ۶۸ مسیر mount شده که در قرارداد نیست   (مثل GET /admin/products، /products/search)
  ۳۶ مسیر در قرارداد که mount نشده        (مثل /catalog/products، /magazine/articles، /cart)
```

این همان «اختلاف مسیر با R-07» است که قبلاً به‌صورت کیفی ثبت شده بود؛ حالا عدد دارد و
**قرارداد و کد دو شکل مختلف API را توصیف می‌کنند** (`/catalog/products` در برابر
`/products`). طبق قاعدهٔ پروژه، بازنویسی خاموش قرارداد ممنوع است — تصمیم با مالک است.

**CI:** job تازهٔ `config-audit` — گیت env **blocking** (`--fail-on-missing`)، گیت routes
فعلاً advisory تا تصمیم R-07.

**۳) فایل‌های env هر unit.** unitها به `/etc/khabavar/<unit>.env` اشاره می‌کردند ولی مخزن
فقط یک `.env.example` داشت — اپراتور باید حدس می‌زد هر unit چه می‌خواهد. سه فایل
`deploy/env/khabavar-{identity,api,worker}.env.example` اضافه شد و `audit-env.py` حالا
بررسی می‌کند هر نامی در آن‌ها واقعاً توسط `config.Load` خوانده می‌شود.

**باگ چهارم که همین کار لو داد:** worker هم `cfg.HTTPAddr` را سرو می‌کند و پیش‌فرضش
`:8080` است — یعنی با unit `api` روی همان پورت تصادم می‌کرد و یکی از دو سرویس بالا
نمی‌آمد. در فایل worker `KH_HTTP_ADDR=:8082` تنظیم شد.

### ۴.۳۸ گزینهٔ ب: کد از قرارداد تبعیت می‌کند — کامیت `feat(contract-aliases)`

تصمیم مالک: مسیرهای قرارداد باید کار کنند (نه اینکه قرارداد بازنویسی شود). aliasها با
**همان handler** اضافه شدند، نه پیاده‌سازی دوم:

```
GET  /catalog/categories            → CategoryTreeHandler
GET  /catalog/products              → SearchProductsHandler
GET  /catalog/products/{id}         → ProductDetailHandler
GET  /magazine/articles             → ListPublishedArticlesHandler
GET  /magazine/articles/{slug}      → GetArticleBySlugHandler
GET  /magazine/topics               → ListTopicsHandler
POST /auth/otp/request              → OTPStartHandler
POST /admin/articles/{id}/relations → AddProductRelationHandler (content.relation.manage)
POST /admin/orders/{id}/cancel      → CancelOrderHandler (order.manage)
```

**شاهد — برابری بایت‌به‌بایت هر جفت مسیر روی باینری واقعی:**

```
PASS catalog/categories    200:1666    PASS magazine/articles        200:793
PASS catalog/products      200:1313    PASS magazine/topics          200:1947
PASS catalog/products/{id} 200:501     PASS magazine/articles/{slug} 200:1202
```

aliasهای ادمین بدون توکن **۴۰۱** می‌دهند (mount شده‌اند و گارد RBAC جلویشان است، نه ۴۰۴)؛
`otp/request` هم مثل `otp/start` پاسخ **۲۰۲** می‌دهد.

**بازرسی مسیرها: ۳۶ → ۲۵.** ۲۵ مسیر باقی‌مانده **قابلیت پیاده‌نشده‌اند**، نه alias: سبد
خرید HTTP، `/me` مشتری، پرداخت (webhook/return)، return-requests، oauth logout،
PATCHهای پنل، shipment events، `GET /catalog/products/{id}/articles`.

### ۴.۳۹ سبد خرید سرورساید (۴ از ۶ مسیر قرارداد) — کامیت `feat(cart-api)`

قرارداد `GET /cart`, `POST /cart/items`, `PATCH|DELETE /cart/items/{id}` را می‌خواست و
هیچ‌کدام پیاده نشده بود. store سبد (`GetCart`, `UpdateCartStatus`) وجود داشت ولی
ساخت/افزودن/تغییر/حذف/ادغام نه.

**افزوده شد:** `CartStore` + `CartView` (با قیمت روز) + سرویس + چهار روت عمومی؛ مهمان با
هدر `X-Guest-Cart-Token` شناسایی می‌شود (طبق قرارداد).

**دو باگ که همین کار لو داد:**

1. **نام جدول قیمت اشتباه بود.** کوئری قیمت را از `price_records` می‌خواند؛ جدول واقعی
   `product_price_records` است. هر خواندن سبد با آیتم، ۵۰۰ می‌داد. (تست PG همین را گرفت،
   نه بازبینی چشمی.)
2. **ایندکس یکتای سبد جزئی است** (`uq_carts_active_session … WHERE status='ACTIVE'`) —
   همان درس باگ ورود مشتری: `ON CONFLICT` باید همان شرط را تکرار کند، وگرنه 42P10.

**قاعدهٔ قیمت:** قیمت هر قلم از **رکورد تأییدشدهٔ روز** خوانده می‌شود، نه از سبد. قلمی که
قیمت تأییدشده ندارد با `availability: UNAVAILABLE` و قیمت صفر برمی‌گردد تا فروشگاه
«قابل فروش نیست» بگوید، نه عدد کهنه.

**شاهد — جریان کامل روی باینری واقعی:**

```
بدون توکن        → 400
GET /cart        → 200 (سبد مهمان ساخته شد)
POST item x2     → 200 (۹۰۰٬۰۰۰ × ۲)
PATCH qty=3      → 200  line_total=2,700,000  subtotal=2,700,000
PATCH qty=0      → 200  items=0   (طبق قرارداد: صفر = حذف)
DELETE ناموجود   → 404
quantity=99      → 400
```

**باقی‌مانده از این خوشه:** `POST /cart/merge` (نیاز به هویت مشتری روی مسیر عمومی —
همان تصمیم احراز هویت) و `POST /checkout` (ایجاد سفارش از سبد + payment intent).

### ۴.۴۰ `POST /checkout` — سبد به سفارش + payment intent — کامیت `feat(checkout)`

ترتیب طبق DB §14.1: اعتبارسنجی سبد و قیمت روز → ایجاد Order با snapshot → تبدیل سبد به
CONVERTED → صدور Payment Intent. رزرو موجودی بعد از تأیید پرداخت انجام می‌شود (PRD O-4)،
نه اینجا.

**قاعده‌های پیاده‌شده:**
- قلم بدون قیمت تأییدشده → **۴۲۲** با فهرست اقلام متأثر (`checkout_unavailable`)، نه قیمت صفر.
- سبد خالی → **۴۰۹** `cart_empty`.
- مهمان بدون `guest_phone` → **۴۰۰** `guest_phone_required` (MA-8).
- سبد **بعد از** ایجاد سفارش تبدیل می‌شود، پس شکست سفارش سبد را سالم می‌گذارد.
- اگر کلاینت `items` بفرستد (بدون `cart_id`)، اول در سبد جمع می‌شوند تا فقط یک مسیر قیمت‌گذاری وجود داشته باشد.

**شاهد — روی باینری واقعی:**

```
سبد خالی        → 409 cart_empty
بدون guest_phone → 400 guest_phone_required   (قبلاً ۵۰۰ بود — رفع شد)
checkout        → 201  order KHA-ORD-2026-000004 · PENDING_PAYMENT · 1,800,000
                  payment intent ba9b266a… · amount 1,800,000 IRR
سبد بعد         → items: 0
checkout دوباره → 409 (سبد تبدیل‌شده، سبد تازه خالی است)
```

**باقی‌مانده:** `Idempotency-Key` روی checkout هنوز اعمال نمی‌شود (جدول idempotency وجود
دارد ولی برای این مسیر سیم نشده) و `redirect_url` درگاه (آداپتر Mock آدرس نمی‌دهد).

### ۴.۴۱ `Idempotency-Key` روی checkout — کامیت `feat(idempotency)`

قرارداد هدر `Idempotency-Key` را **required** می‌داند و ADR-ARCH-008 می‌گوید درخواست
تکراری باید همان نتیجه را برگرداند. تا امروز فقط `payment_attempts` ستون idempotency داشت،
نه خود درخواست — یعنی یک retry شبکه‌ای می‌توانست **دو سفارش** بسازد.

**افزوده شد:** مهاجرت `000015_idempotency` + `IdempotencyStore` + wrapper
`commerce.Idempotent`:

- بدون هدر → **۴۰۰** `idempotency_key_required`
- اولین درخواست کلید را claim می‌کند، اجرا می‌شود و پاسخش (status + body) ذخیره می‌شود
- retry با همان کلید و همان body → **همان پاسخ، بایت‌به‌بایت** (هدر `Idempotent-Replay: true`)
- همان کلید با body متفاوت → **۴۰۹** `idempotency_conflict`
- همان کلید در حال اجرا → **۴۰۹** `idempotency_in_flight` (نه سفارش دوم)
- فقط پاسخ‌های موفق ذخیره می‌شوند؛ خطا قابل retry می‌ماند

**هش canonical:** بدنه قبل از هش به JSON دوباره کد می‌شود، پس ترتیب کلید و فاصله‌ها
درخواست را «متفاوت» نشان نمی‌دهد (وگرنه یک retry بی‌ضرر ۴۰۹ می‌گرفت).

**شاهد — روی باینری واقعی:**

```
بدون کلید          → 400 idempotency_key_required
اولین درخواست      → 201  KHA-ORD-2026-000005
retry همان کلید    → 201  KHA-ORD-2026-000005   ← همان سفارش
همان کلید، body دیگر → 409 idempotency_conflict
تعداد سفارش با آن شماره در DB → 1   ← سفارش دومی ساخته نشد
```

### ۴.۴۲ سطح مشتری `/me` + دفترچهٔ آدرس — کامیت `feat(customer-me)`

قرارداد `GET /me`، `/me/addresses` (GET/POST/PATCH/DELETE) را می‌خواست و هیچ‌کدام نبود.
مسیرهای عمومی هیچ احراز هویتی نداشتند، پس اول یک mount تازه ساخته شد:

- `apiMux.customer` با `authorization.AuthenticateCustomer`: توکن باید به **مشتری** حل شود؛
  توکن کارمند صراحتاً رد می‌شود تا سطح مشتری درِ پشتی هویت کارمند نباشد (و برعکس).
- `CustomerStore` + `DBCustomerStore`: پروفایل، فهرست/ایجاد/ویرایش/حذف آدرس.

**قاعده‌های پیاده‌شده:** فقط یک آدرس پیش‌فرض (آدرس جدید پیش‌فرض، قبلی را در همان تراکنش
پاک می‌کند)؛ هر عملیات آدرس با `customer_id` محدود است، پس آیدی یک مشتری دیگر ۴۰۴ می‌دهد.

**شاهد — با ورود واقعی OTP و توکن مشتری:**

```
GET /me            → 200  (+989…0777، phone_verified: true)
POST address       → 201
GET addresses      → 200  count: 1  default: true
PATCH address      → 200
DELETE address     → 204
DELETE دوباره      → 404
توکن ادمین روی /me → 401   ← سطح مشتری درِ پشتی نیست
```

**بازرسی مسیرها: ۲۰ → ۱۵.** `/me/orders` (فهرست سفارش‌های مشتری) و `POST /cart/merge`
باقی مانده‌اند؛ merge حالا فقط به یک handler نیاز دارد چون هویت مشتری موجود است.

### ۴.۴۳ ادغام سبد مهمان + سفارش‌های مشتری — کامیت `feat(cart-merge)`

دو مسیر باقی‌ماندهٔ خوشهٔ مشتری:

- **`POST /cart/merge`** (MA-8): سبد مهمان به سبد مشتری ادغام می‌شود. `FindCartBySession`
  عمداً **نمی‌سازد**: توکنی که سبد ندارد no-op است، نه یک سبد خالی جدید. ادغام دوباره
  تکرار نمی‌کند (سبد مهمان CONVERTED می‌شود) — تست همین را ثابت می‌کند.
- **`GET /me/orders`**: فهرست سفارش‌های خود مشتری، با `customer_id` در SQL محدود شده؛
  تست ثابت می‌کند مشتری دیگر صفر سفارش می‌بیند.

**بازرسی مسیرها: ۱۵ → ۱۳.**

**شاهد (PG، روی schema واقعی):** `TestPG_MergeGuestCartAndCustomerOrders` — ادغام،
بی‌اثر بودن ادغام دوم، `FindCartBySession` برای توکن ناشناخته `nil` (بدون ساخت سبد)، و
جداسازی سفارش‌های دو مشتری.

### ۴.۴۴ webhook درگاه + بازگشت مرورگر — کامیت `feat(payments-webhook)`

بدون این دو، هیچ سفارشی از `PENDING_PAYMENT` خارج نمی‌شد.

**`POST /payments/webhook`:**
- امضای `X-Provider-Signature` روی **بدنهٔ خام** با HMAC-SHA256 بررسی می‌شود (کدگذاری
  دوبارهٔ JSON امضا را می‌شکند). کلید خالی → **fail closed**، نه fail open.
- اعلان خام هرگز مبنای تأیید نیست؛ فقط می‌گوید کدام تراکنش را نگاه کنیم. تأیید از مسیر
  `ConfirmPaymentForOrder` می‌رود که payment attempt و رویداد outbox را در یک تراکنش می‌نویسد.
- مرجع ناشناخته → **۲۰۲** (درگاه دست از retry بردارد، ولی چیزی تأیید نشده).
- **retry درگاه → ۲۰۰ `already_confirmed`**؛ اگر ۵۰۰ می‌دادیم درگاه تا ابد تکرار می‌کرد.

**`GET /payments/return`:** فقط مرورگر را به صفحهٔ سفارش هدایت می‌کند (۳۰۲) و خودش چیزی
تأیید نمی‌کند. مقصد از config می‌آید تا open redirect نشود.

**دو باگ حین کار:**
1. `ConfirmPayment(intentID)` یک stub است که همیشه خطا می‌دهد؛ مسیر واقعی
   `ConfirmPaymentForOrder` است. webhook اول ۵۰۰ می‌داد.
2. retry درگاه ۵۰۰ می‌گرفت (سفارش قبلاً PAID بود) — حالا ۲۰۰.

**شاهد — روی باینری واقعی:**

```
بدون امضا      → 401
امضای غلط      → 401
امضای درست     → 200 {"status":"confirmed"}   → وضعیت سفارش: PAID
retry همان اعلان → 200 (already_confirmed)
مرجع ناشناخته  → 202
/payments/return → 302 https://shop.khabavar.ir/order/KHA-ORD-2026-000008
```

**بازرسی مسیرها: ۱۳ → ۱۱.**

### ۴.۴۵ محموله و رویدادهای رهگیری — کامیت `feat(shipments)`

دو مسیر قرارداد: `POST /admin/fulfillments/{id}/shipments` و
`POST /admin/shipments/{id}/events`.

- `CreateShipmentHandler` از قبل نوشته شده بود ولی **mount نشده بود** — فقط وصل شد.
- `Service.AddShipmentEvent` تازه: رخداد رهگیری را به تایم‌لاین اضافه می‌کند **بدون**
  تغییر وضعیت محموله. تغییر وضعیت باید از `TransitionShipment` بگذرد تا ماشین حالت
  سالم بماند.
- منبع رخداد قبل از هر نوشتن اعتبارسنجی می‌شود (CHECK فقط MANUAL/CARRIER_API/WEBHOOK را
  می‌پذیرد)؛ رخداد بی‌کد رد می‌شود.

**شاهد (PG):** `TestPG_AddShipmentEvent` — رخداد ثبت می‌شود، وضعیت محموله دست‌نخورده
می‌ماند، و منبع نامعتبر پیش از درج رد می‌شود (تعداد رخدادها تغییر نمی‌کند).

**بازرسی مسیرها: ۱۱ → ۹.**

### ۴.۴۶ ویرایش مقاله + بازبینی در پنل — کامیت `feat(article-edit)`

- **`PATCH /admin/articles/{id}`**: `Service.UpdateArticle` + `DBStore.UpdateArticle`.
  فیلدها pointer-based‌اند (PATCH نباید فیلدی را که ذکر نشده خالی کند)، **وضعیت قابل
  ویرایش نیست** (گردش کار مسیرهای محافظت‌شدهٔ خودش را دارد)، و slug/body خالی رد می‌شود.
- **`POST /admin/articles/{id}/review`**: قرارداد یک endpoint بازبینی می‌خواست؛ به‌جای
  ماشین حالت دوم، `decision` روی همان انتقال‌های محافظت‌شده نگاشت می‌شود
  (`submit`/`approve`/`reject`). ویرایشگرِ تأییدکننده از **actor احرازشده** می‌آید، نه از بدنه.

**شاهد (PG):** `TestPG_UpdateArticleAndReview` — ویرایش عنوان با slug دست‌نخورده، وضعیت
بعد از PATCH همان DRAFT، رد شدن slug/body خالی، رد شدن submit دوباره روی IN_REVIEW، و
چرخهٔ submit → approve → reject.

**بازرسی مسیرها: ۹ → ۷.**

### ۴.۴۷ مقالات مرتبط با محصول — کامیت `feat(product-articles)`

`GET /catalog/products/{id}/articles`: خواندن مرتبط با محصول در صفحهٔ محصول.

- `DBStore.ListPublishedArticlesForProduct` — فیلتر `status = 'PUBLISHED'` **در SQL** است،
  نه در فراخواننده: ویترین هرگز نباید از طریق رابطهٔ محصول به یک DRAFT برسد.
- `ListRelationsForProduct` از قبل بود؛ این جهتِ معکوس (رابطه → مقاله) را اضافه می‌کند.

**شاهد (PG):** `TestPG_PublishedArticlesForProduct` — یک مقالهٔ منتشرشده و یک پیش‌نویس،
هر دو مرتبط با همان محصول؛ خروجی فقط منتشرشده را برمی‌گرداند.

**بازرسی مسیرها: ۷ → ۶** (بقیه: `PATCH /admin/suppliers`، `PATCH /admin/users`،
`POST /admin/users`، `POST /admin/orders/{}/fulfillments`، `POST /oauth/logout`،
`POST /orders/{}/return-requests`).

### ۴.۴۸ fulfillment از مسیر سفارش — کامیت `feat(order-fulfillments)`

`POST /admin/orders/{id}/fulfillments` — حلقهٔ گمشدهٔ زنجیرهٔ عملیاتی.

- handler موجود، `order_id` را از **بدنه** می‌گرفت؛ قرارداد آن را در **مسیر** می‌خواهد.
  در نسخهٔ مسیرمحور، مسیر مرجع است: اگر بدنه `order_id` متفاوتی بفرستد **۴۰۰** می‌دهد،
  نه اینکه بی‌صدا نادیده بگیرد — وگرنه می‌شد سفارش اشتباه را fulfillment کرد.
- `CreateFulfillmentOrder` از قبل بود و تست PG داشت؛ این فقط لایهٔ HTTP + mount است.

**شاهد:** `TestCreateOrderFulfillmentRejectsContradictingBody` (بدنهٔ متناقض → ۴۰۰، و
گارد **قبل از** هر فراخوانی سرویس فعال است — با سرویس `nil` تست شده).

**بازرسی مسیرها: ۶ → ۵.**

### ۴.۴۹ درخواست مرجوعی مشتری — کامیت `feat(return-requests)`

`POST /orders/{id}/return-requests` — تنها مسیر مشتری باقی‌مانده و از نظر مالی حساس.

**مهاجرت `000016_return_requests`:** جدول تازه با CHECK روی وضعیت
(`REQUESTED|APPROVED|REJECTED|RECEIVED|REFUNDED`) و **ایندکس یکتای جزئی** روی
`(order_id) WHERE status IN ('REQUESTED','APPROVED','RECEIVED')` — مشتری نمی‌تواند
درخواست‌های موازی روی یک سفارش بسازد، ولی تاریخچه بعد از تصمیم باز می‌ماند.

**قاعده‌ها:** مرجوعی فقط بعد از پرداخت (`PAID`/`IN_FULFILLMENT`/`SHIPPED`/`COMPLETED`)؛
سفارش با `customer_id` محدود است (سفارش مشتری دیگر → ۴۰۴)؛ دلیل الزامی؛ تعداد مثبت؛
و نوشتن در **Audit** با actorType `CUSTOMER` — خطای audit برگردانده می‌شود، نه بلعیده.

**Idempotency:** مسیر پشت `Idempotent(s.idem, "return-request", …)` است تا دوبار کلیک
دو درخواست نسازد.

**شاهد (PG):** `TestPG_ReturnRequest` — رد شدن مرجوعی روی `PENDING_PAYMENT`، رد شدن دلیل
خالی و تعداد صفر، رد شدن مشتری غریبه، ساخت موفق، و **۴۰۹‌مانند** شدن درخواست دوم از
طریق ایندکس یکتا (`ErrReturnAlreadyOpen`).

**بازرسی مسیرها: ۶ → ۴.**

### ۴.۵۰ مدیریت حساب‌های کارمند — کامیت `feat(admin-users)`

`POST /admin/users` + `PATCH /admin/users/{id}`.

- **نقش‌ها با کد از جدول seed حل می‌شوند، نه با id هاردکد** (IAM §۴). کد ناشناخته کل
  تراکنش را برمی‌گرداند، پس یک غلط تایپی حسابِ بی‌نقش نمی‌سازد.
- `PATCH` با `roles: nil` نقش‌ها را **دست نمی‌زند**؛ تغییر نام نباید بی‌صدا مجوزها را
  پاک کند. `roles` غیر-nil جایگزین کامل مجموعه است.
- گذرواژه ۱۲ کاراکتر حداقل، bcrypt cost 12. گذرواژه در `PATCH` **قابل تغییر نیست** —
  تغییر اعتبارنامه جریان audited خودش را می‌خواهد.
- ایمیل تکراری → **۴۰۹**؛ id ناشناخته → **۴۰۴**.

**شاهد (PG):** `TestPG_AdminUserLifecycle` — ساخت با نقش، ایمیل تکراری، کد نقش ناشناخته
(و **صفر** حساب یتیم باقی می‌ماند)، تغییر نام با حفظ نقش، جایگزینی نقش + SUSPEND، و ۴۰۴.

**بازرسی مسیرها: ۴ → ۲** (`PATCH /admin/suppliers`، `POST /oauth/logout`).

### ۴.۵۱ ویرایش تأمین‌کننده + خروج — کامیت `feat(supplier-patch-logout)`

آخرین دو مسیر قرارداد.

- **`PATCH /admin/suppliers/{id}`**: `Service.UpdateSupplier` + `DBStore.UpdateSupplier`.
  فیلدها pointer-based، نام الزامی، و **وضعیت قابل ویرایش نیست** (مسیر محافظت‌شدهٔ
  `/{id}/status` را دارد). id ناشناخته → ۴۰۴، نه نوشتن خاموش.
- **`POST /oauth/logout`**: کوکی `kh_session` را پاک می‌کند و **۲۰۴ برای همه** می‌دهد —
  حتی بدون نشست. یک logout که لو بدهد نشستی وجود داشت یا نه، یک oracle است؛ و کلاینت
  باید بتواند idempotent خارج شود.

**شاهد (PG):** `TestPG_UpdateSupplier` — نام/تلفن نوشته می‌شوند، **وضعیت با ویرایش
پروفایل تغییر نمی‌کند**، نام خالی رد می‌شود، id ناشناخته → `ErrSupplierNotFound`.

**بازرسی مسیرها: ۲ → ۰.** بخش «DECLARED BUT NOT MOUNTED» از خروجی
`scripts/audit-routes.py` حذف شد — یعنی هیچ مسیر قراردادی بدون mount نمانده
(اسکریپت آن بخش را فقط وقتی چاپ می‌کند که فهرست خالی نباشد).

### ۴.۵۲ دروازهٔ operationId + توصیف عملیات‌ها — کامیت `docs(openapi)`

**یافتهٔ اولیه:** ادعای «operationId وجود ندارد» **غلط بود**. قرارداد از قبل ۵۸
`operationId` یکتا و camelCase داشت؛ ولی قاعدهٔ spectral برای آن `off` بود و کامنتش
می‌گفت «قرارداد operationId ندارد». یعنی دروازه خاموش بود، نه اینکه داده نبود.

- قاعده به `error` تغییر کرد + `operation-operationId-unique: error`.
- **اثبات اینکه دروازه واقعاً می‌گیرد:** یک operationId عمداً تکراری شد →
  `error operation-operationId-unique` و خروجی غیرصفر؛ سپس فایل بازگردانده شد.
- **۲۵ توصیف فارسی** برای عملیات‌هایی که `description` نداشتند اضافه شد
  (هشدارهای `operation-description` از ۲۹ به صفر).

**وضعیت lint:** `0 errors, 4 warnings` — چهار کامپوننت تعریف‌شده ولی بی‌استفاده
(`AcceptLanguage`، `SessionInfo`، `RefundStatus`، `ArticleStatus`). **دست نزدم** چون
انتخاب بین «وصل کردن» و «حذف» یک تصمیم قراردادی است و طبق قاعدهٔ مالک نیاز به تأیید
دارد؛ در بخش ۵ ثبت شد.

### ۴.۵۳ باگ سبد مشتری + تست لایهٔ HTTP سبد — کامیت `fix(cart)`

**یافته:** چهار handler سبد (`Cart`, `AddToCart`, `UpdateCartItem`, `RemoveCartItem`)
`customerID` را **رشتهٔ خالی هاردکد** کرده بودند:

```go
svc.AddToCart(r.Context(), "", r.Header.Get(guestTokenHeader), …)
```

یعنی مشتری واردشده هرگز سبد خودش را نمی‌دید؛ هر درخواست سبد **مهمان** بود و بدون هدر
`X-Guest-Cart-Token` با ۴۰۰ رد می‌شد. سبد مشتری عملاً غیرقابل استفاده بود.

**رفع:** `authorization.CustomerID(r.Context())` جای `""` در هر چهار نقطه.

**تست تازه:** `TestPG_CartHTTPHandlers` — افزودن، مشاهده، تغییر تعداد، حذف، و مسیرهای
مشتری، همه از طریق handler روی schema واقعی. این تست همان چیزی است که باگ را گرفت:
تست سرویس نمی‌توانست ببیند handler هویت را دور می‌ریزد.

**دو نکته که تست روشن کرد:**
- `quantity: 0` در قرارداد یعنی «حذف قلم»، نه خطا؛ تست با `-1` نوشته شد.
- handler از `r.PathValue("id")` می‌خواند، نه `cartItemId`.

**پوشش commerce: ۵۵.۰٪ → ۶۱.۸٪** (آستانهٔ R-CI §۳ برای مالی ۹۰٪ است — ادامه دارد).

### ۴.۵۴ باگ checkout + تست لایهٔ HTTP پول — کامیت `fix(checkout)`

**همان کلاس باگ، نقطهٔ دوم:** `CheckoutHandler` هم `customerID` را `""` هاردکد کرده بود.
یعنی مشتری واردشده checkout می‌زد و سرویس سبد **مهمان** را می‌دید → `409 cart_empty`.

**جاروب سیستماتیک:** `grep -rn 'r.Context(), ""' internal/` — بعد از دو رفع، **صفر**
نقطهٔ باقی‌مانده.

**تست‌های تازه (۳۱۲ تست):**
- `TestPG_CheckoutHandlerUsesCustomerCart` — سبد مشتری از طریق handler پر می‌شود، checkout
  `۲۰۱` می‌دهد، و **سفارش به همان مشتری تعلق دارد** (نه مهمان). checkout دوم روی سبد خالی
  → `409`.
- `TestPG_RefundHTTPHandlers` — بازپرداخت بدون دلیل → ۴۰۰، مبلغ صفر → ۴۰۰، بازپرداخت
  جزئی ۳۰٬۰۰۰ → ۲۰۱، تاریخچه → ۲۰۰، reconciliation → ۲۰۰، سفارش ناشناخته → 4xx (نه ۵۰۰).

**دو گیت که تست‌ها روشن کردند:** checkout خطی را که **قیمت تأییدشده ندارد** با
`422 price_unavailable` رد می‌کند، و `product_price_records.approved_by` به
`admin_users` ارجاع دارد — پس تست باید یک حساب کارمند واقعی بسازد.

**پوشش commerce: ۶۱.۸٪ → ۶۷.۸٪** (آستانهٔ مالی ۹۰٪).

### ۴.۵۵ جاروب کلاس باگ هویت + audit موجودی — کامیت `feat(inventory-audit)`

**جاروب سیستماتیک (اسکریپت، نه چشم):** همهٔ mountهای `api.customer` (۸) و
`api.admin` (۷۱) استخراج و بدنهٔ handlerشان بررسی شد.

- **مسیرهای مشتری:** تنها موردِ مشکوک، `return-requests` بود که false positive است —
  handler داخل `Idempotent(...)` پیچیده و اسکریپت بدنهٔ wrapper را دیده بود؛ خودِ handler
  `CustomerID` را می‌خواند. **هیچ مسیر مشتری بدون هویت نماند.**
- **مسیرهای ادمین:** **۶۷ از ۷۱** handler هرگز به actor دست نمی‌زنند. یعنی IAM §۷
  (هفت دستهٔ حساس) عملاً فقط برای بازپرداخت و بخشی از کاتالوگ اجرا می‌شد.

**گام اول از آن ۶۷: تغییر موجودی.** ماژول `inventory` هیچ auditor نداشت:

- `AuditRecorder` + `SetAuditor` (همان شکل commerce/catalog، بدون import چرخه‌ای)
- `AdjustStock` بعد از ثبت movement رکورد audit می‌نویسد — **ترتیب مهم است**: audit بعد از
  movement، تا تریل ادعای تغییری نکند که ثبت نشده
- خطای audit **برگردانده می‌شود**، نه بلعیده
- در `main.go` وصل شد (`invSvc.SetAuditor(auditor)`)

**شاهد:** `TestAdjustStockWritesAudit` — یک رکورد audit با `action=inventory.adjust` و
actor درست؛ `delta=0` رد می‌شود و **audit نمی‌شود**؛ و خطای auditor به فراخواننده برمی‌گردد.

**یک باگ حین کار:** نسخهٔ اول بلوک audit را با الگوی متنی اشتباه در `ReceiveStock` جا
داد (جایی که `delta`/`notes` وجود ندارد) — کامپایل گرفت و اصلاح شد.

### ۴.۵۶ audit تغییر وضعیت سفارش — کامیت `feat(order-audit)`

دستهٔ دوم از هفت دستهٔ IAM §۷: **تغییر وضعیت سفارش**.

- `Service.auditOrder(ctx, action, orderID, from, to, reason)` — یک نقطهٔ واحد، با
  `shared.ActorID(ctx)` برای actor و **خطای برگشتی** (نه بلعیده).
- دو نقطهٔ فراخوانی: `ConfirmPaymentForOrder` (`order.paid`) و `CancelOrder`
  (`order.cancelled` با دلیل در payload).
- audit بعد از نوشتن وضعیت و بعد از enqueue نوشته می‌شود، تا تریل ادعای تغییری نکند که
  ثبت نشده.

**شاهد (PG):** `TestPG_OrderStatusAudit` — هر دو اقدام در audit ظاهر می‌شوند
(`order.cancelled`، `order.paid`) و خطای auditor از `CancelOrder` به فراخواننده برمی‌گردد.

**پوشش دسته‌های §۷ تا اینجا:** بازپرداخت ✓، تغییر موجودی ✓، تغییر وضعیت سفارش ✓،
بخشی از کاتالوگ/قیمت (helper موجود در `pricing/admin.go`). باقی: هزینهٔ خرید، تأیید تأمین،
شرایط ارسال.

### ۴.۵۷ سه دستهٔ باقی‌ماندهٔ IAM §۷ — کامیت `feat(audit-seven)`

با این دور، **هر هفت دستهٔ §۷** پوشش داده شد.

| دسته | ماژول | action |
|---|---|---|
| بازپرداخت | commerce | `order.refund.issued` |
| تغییر موجودی | inventory | `inventory.adjust` |
| تغییر وضعیت سفارش | commerce | `order.paid` / `order.cancelled` |
| قیمت فروش / کاتالوگ | catalog + pricing | helper موجود |
| هزینهٔ خرید | suppliers | `supplier.price` |
| تأیید/تعلیق تأمین | suppliers | `supplier.status` |
| شرایط/وضعیت ارسال | fulfillment | `shipment.status` |

**الگوی یکسان در هر سه ماژول:** `AuditRecorder` + `SetAuditor` (بدون import چرخه‌ای)،
audit **بعد از** نوشتن رکورد اصلی، و **خطای برگشتی** نه بلعیده.

**جزئیات مهم:**
- `UpdateSupplierVariantPrice` قیمت قبلی را **قبل از** تغییر می‌خواند تا audit «قبل/بعد»
  داشته باشد؛ audit بدون «قبل» در عمل قابل بازرسی نیست.
- `TransitionShipmentStatus` قبل از هر نوشتن، منبع رخداد و مجاز بودن انتقال را بررسی
  می‌کند؛ انتقال رد‌شده **audit نمی‌شود**.
- هر سه در `main.go` وصل شدند.

**شاهد:** `TestSupplierAudit`، `TestShipmentTransitionAudit`، `TestPG_OrderStatusAudit`،
`TestAdjustStockWritesAudit` — هر چهار مورد علاوه بر ثبت رکورد، **خطای auditor را به
فراخواننده برمی‌گردانند** و **عملیات رد‌شده را audit نمی‌کنند**.

**تست‌ها: ۳۱۲ → ۳۱۶.**

---

### ۴.۵۸ بازنویسی §۵ و هم‌ترازی PROJECT_MANIFEST — کامیت `docs(status)`

**چرا:** §۵ سند سه ادعای کهنه داشت که خودم در همین دوره با خروجی ابزار رد کردم:

| ادعای کهنه | واقعیت |
|---|---|
| «operationId وجود ندارد» | ۵۸ یکتا از قبل بود؛ قاعده خاموش بود |
| «magazine و admin هنوز scaffold» | هر دو ساخته و بیلد سبز |
| «audit فقط برای بازپرداخت» | هفت از هفت دستهٔ §۷ پوشش دارد |

سندی که وضعیت غلط گزارش کند همان چیزی است که R-19 §۹ منع می‌کند، حتی وقتی ادعا
«محافظه‌کارانه» به نظر می‌رسد.

**§۵ بازنویسی شد** به دو بخش «بسته‌شده (با شاهد)» و «باز (به ترتیب اولویت)»، با
اعداد اندازه‌گیری‌شده و نام ابزاری که هر عدد را تولید می‌کند.

**`PROJECT_MANIFEST.md` به v1.2:** migrationها ۱۱→۱۶، تست‌ها ۲۳۰+→۳۱۶، endpointها
~۶۰→۵۸/۵۸، فاز P2 و فرانت‌اند از «باقی‌مانده» به «تکمیل‌شده»، و بلوک تازهٔ «اعداد
تأییدشده» با منبع هر عدد. **پوشش تست صریحاً «برآورده نشده» علامت خورد** — نه با
عبارت نرم.

---

### ۴.۵۹ لایهٔ HTTP موجودی + دو باگ — کامیت `fix(inventory-http)`

لایهٔ HTTP `inventory` **کاملاً بدون تست** بود (۰٪ روی همهٔ handlerها). تست تازه
(`TestPG_InventoryHTTPHandlers`) کل سطح را می‌راند: انبار، رسید، مانده، تعدیل، چرخهٔ
رزرو (رزرو → مانده → آزادسازی → تبدیل) و دفتر حرکات.

**دو باگ که تست گرفت:**

1. **هویت از بدنه، نه از context.** `ReceiveStockHandler` و `AdjustStockHandler`
   `created_by` را از **بدنهٔ درخواست** می‌خواندند. یعنی پنل می‌توانست یک تغییر موجودی را
   به نام شخص دیگری ثبت کند — همان کلاس باگی که در سبد و checkout پیدا شد، این بار در
   مسیر حسابرسی‌شده. حالا `shared.ActorID(r.Context())` مرجع است و نبودِ actor → **۴۰۱**.
2. **کلیدهای PascalCase در پاسخ.** `StockBalance` هیچ JSON tag نداشت، پس API
   `OnHandQuantity` برمی‌گرداند نه `on_hand_quantity` — ناسازگار با بقیهٔ API. tagها
   اضافه شد.

**سه واقعیت schema که تست روشن کرد:** `reference_id` و `order_item_id` هر دو UUID‌اند
(مرجع انسانی مثل `PO-1` با 22P02 رد می‌شود)، رزرو بدون `expires_at` رد می‌شود، و
`stock_movements.created_by` کلید خارجی به `admin_users` دارد.

**پوشش inventory: ۴۷.۲٪ → ۶۲.۸٪** (آستانهٔ مالی ۹۰٪).

---

### ۴.۶۰ لایهٔ HTTP قیمت‌گذاری — کامیت `test(pricing-http)`

هفت handler قیمت‌گذاری ۰٪ پوشش داشتند. تست تازه (`TestPG_PricingHTTPHandlers`) کل سطح
پنل را می‌راند: رکورد هزینه، رکورد قیمت، تأیید، دو فهرست و اقتصاد واحد.

**پیش‌بینی من غلط بود و این خوب است:** انتظار داشتم همان باگ «هویت از بدنه» را اینجا هم
پیدا کنم؛ `pricing` از قبل درست بود — `shared.ActorID(r.Context())` برای `created_by` و
`approved_by`، با کامنت صریح ارجاع به R-05 §۹.۲. **هیچ باگی نبود.**

**یک ادعای من که قرارداد رد کرد:** فرض کردم قیمت زیر بهای تمام‌شده باید رد شود. قرارداد
(R-14 خط ۴۹۲) می‌گوید سود مشارکت **می‌تواند صفر یا منفی شود** و BR-06 فقط **هشدار و
بازبینی** است؛ دروازه، تأیید انسانی است. تست به‌جای تحمیل فرض من، رفتار مستند را
تثبیت می‌کند: `201` با `warnings` حاوی BR-06.

**شاهد:** تأیید قیمت، `approved_by` را از **نشست** برمی‌گرداند (نه از بدنه)؛ تأیید دوباره
۴xx می‌دهد؛ سه مسیر خواندنی ۲۰۰ می‌دهند.

**پوشش pricing: ۶۱.۷٪ → ۷۹.۹٪** (آستانهٔ مالی ۹۰٪).

---

### ۴.۶۱ idempotency و پرداخت — تست خودکار — کامیت `test(commerce-money)`

سه مسیری که تا حالا فقط **دستی** تأیید شده بودند (با curl) حالا تست خودکار دارند:

**`TestPG_IdempotencyMiddleware`** — میدل‌وری که همهٔ عملیات پولی را می‌پوشاند:
- بدون `Idempotency-Key` → ۴۰۰ و **handler اجرا نمی‌شود**
- تکرار با همان کلید → همان پاسخ + هدر `Idempotent-Replay: true` و **handler دوباره
  اجرا نمی‌شود**
- همان کلید با بدنهٔ متفاوت → **۴۰۹** (نه عملیات دوم بی‌صدا)
- خطای handler کلید را **قابل retry** می‌گذارد

**`TestPG_PaymentHandlers`** — webhook و return:
- بدون امضا → ۴۰۱، امضای غلط → ۴۰۱، امضای درست → ۲۰۰ و سفارش `PAID`
- retry درگاه → ۲۰۰ `already_confirmed`
- `/payments/return` → ۳۰۲ به صفحهٔ سفارش؛ بدون `order_number` → ۴۰۰

**یک محدودیت که تست روشن کرد:** ستون `idempotency_keys.response_body` از نوع **jsonb**
است، پس Postgres فاصله‌ها را نرمال می‌کند. کامنت کد می‌گفت «پاسخ اول را عیناً replay
می‌کند» — **عیناً در سطح بایت درست نیست**، فقط معنایی. تست به‌جای ادعای بایت‌به‌بایت،
برابری معنایی را بررسی می‌کند و محدودیت در کد مستند شد.

**پوشش commerce: ۶۸.۳٪ → ۷۴.۳٪** (آستانهٔ مالی ۹۰٪).

---

### ۴.۶۲ دو stub که تله بودند — کامیت `fix(commerce-stubs)`

پروفایل پوشش دو تابع **مرده** را نشان داد که هر دو همیشه خطا برمی‌گرداندند:

1. **`ListOrdersByCustomerHandler`** — stub با پیام «not yet implemented (P4)». مسیر واقعی
   `/me/orders` از قبل با `MyOrdersHandler` پیاده شده بود، پس این تابع **هیچ فراخواننده‌ای
   نداشت**. حذف شد: کد مرده‌ای که خطا برمی‌گرداند، تلهٔ نفر بعدی است.
2. **`ConfirmPayment(intentID)`** — stub با پیام «use ConfirmPaymentForOrder». این یکی
   حذف نشد، **پیاده شد**: intent را به سفارشش حل می‌کند (`OrderIDForIntent`) و به
   `ConfirmPaymentForOrder` واگذار می‌کند، تا **یک** مسیر تأیید وجود داشته باشد نه دو.

**شاهد:** `TestPG_ConfirmPaymentByIntent` — تأیید با شناسهٔ intent سفارش را `PAID` می‌کند؛
intent ناشناخته و رشتهٔ خالی → `ErrPaymentIntentNotFound` (نه panic، نه ۵۰۰).
`TestPG_ListOrdersHandler` — صفحه‌بندی نامعتبر (`page=0&page_size=9999`) به پیش‌فرض
۱/۲۰ برمی‌گردد و فیلتر بی‌نتیجه لیست خالی می‌دهد، نه خطا.

**پوشش commerce: ۷۴.۳٪ → ۷۶.۳٪** · **پوشش SQL: ۹۰/۹۰**.

---

### ۴.۶۳ خطای مرجوعی که ۵۰۰ می‌داد — کامیت `fix(return-error-map)`

تست handler مرجوعی نشان داد **درخواست بد کلاینت ۵۰۰ می‌گیرد**: `ErrReturnReasonRequired`
و بقیهٔ sentinelهای مرجوعی در `statusForError` نگاشت نشده بودند، پس به شاخهٔ `default`
(۵۰۰) می‌افتادند.

این فقط «کد اشتباه» نیست: خطای کلاینت در سیگنال خطای سرور می‌نشیند و هشدارهای 5xx را
بی‌معنا می‌کند.

**نگاشت اضافه شد:** `ErrReturnReasonRequired`/`ErrReturnQuantityInvalid` → **۴۰۰**،
`ErrReturnNotAllowed`/`ErrReturnAlreadyOpen` → **۴۰۹**.

**تست‌های تازه:**
- `TestPG_CreateReturnRequestHandler` — بدون دلیل ۴۰۰، مشتری غریبه ۴۰۴، ساخت ۲۰۱،
  درخواست دوم ۴۰۹.
- `TestPG_CheckoutErrorBranches` — بدون آدرس ۴۰۰، سبد خالی ۴۰۹، قلم بدون قیمت تأییدشده
  **۴۲۲** با فهرست اقلام.
- `TestCheckoutErrorMessages` — `ErrCheckoutUnavailable.Error()` و `MarshalPayload`
  (شامل حالت خطا برای مقدار غیرقابل marshal).

**پوشش commerce: ۷۶.۳٪ → ۷۸.۹٪.**

---

### ۴.۶۴ شاخه‌های خطا و مرزهای دسترسی — کامیت `test(commerce-branches)`

سه تست تازه، هر کدام یک دستهٔ شاخه که مسیر خوش‌بینانه نمی‌بیند:

- **`TestCartErrorCodes`** — شش کد ماشین‌خوان سبد (`cart_token_required`،
  `cart_item_not_found`، `invalid_quantity`، …) و حالت «خطای ناشناخته → صفر». کدی که
  بی‌صدا عوض شود، کلاینت را می‌شکند بدون اینکه بیلد بشکند.
- **`TestPG_MyOrdersHandler`** — صفحه‌بندی نامعتبر (`page=-5&page_size=1000`) به پیش‌فرض
  برمی‌گردد؛ مشتری دیگر **صفر** سفارش می‌بیند؛ و درخواست **بدون هویت** صفر سفارش
  برمی‌گرداند، نه سفارش‌های همه.
- **`TestPG_WebhookBranches`** — بدنهٔ خراب ۴۰۰، مرجع غایب ۴۰۰، مرجع ناشناخته ۲۰۲، و
  **کلید خالی → ۴۰۱ (fail closed)**.

**پوشش commerce: ۷۸.۹٪ → ۸۰.۳٪.**

**باقی‌مانده تا ۹۰٪:** شاخه‌های خطای `Cart`/`AddToCart`/`MergeGuestCart` و
`MergeCarts` (۶۰–۶۷٪) — عمدتاً مسیرهای «سبد منقضی»، «قلم متعلق به سبد دیگر» و
«ادغام سبد با صفر قلم».

---

### ۴.۶۵ بررسی ریز: ورود کارمند روی origin API + کلیدهای PascalCase — کامیت `fix(api-origin)`

بررسی سیستماتیک با اسکریپت (نه چشم) سه یافته داد:

**۱) ورود کارمند روی واحد API وجود نداشت.** `POST /api/v1/auth/admin/login` فقط در
`runIdentity` (پورت جدا) ثبت شده بود؛ پنل اما از همان origin که `/api/v1` را سرو می‌کند
لاگین می‌کند. نتیجه: **در استقرار تک‌واحدی API، ورود پنل ۴۰۴ می‌داد** — و این را
`audit-routes.py` نمی‌دید، چون مسیر در قرارداد نیست (فقط handler بود).

رفع: `identity.Service` در واحد API ساخته شد و مسیر با `api.public` mount شد.
**شاهد:** `POST /auth/admin/login → ۲۰۰`، توکن ۶۴ کاراکتری، و `/admin/orders` و
`/admin/stock` با همان توکن **۲۰۰**.

**۲) نشت کلیدهای PascalCase در ۱۰ ساختار پاسخ.** ساختارهایی که JSON tag نداشتند
(`Movement`, `Reservation`, `Warehouse`, `Supplier`, `Evaluation`, `Shipment`,
`FulfillmentOrder`, `Refund`, `Cart`, `ProductArticleRelation`) کلید Go-style
برمی‌گرداندند. **شاهد زنده قبل از رفع:**
`{"data":[{"ID":…,"WarehouseID":…,"MovementType":…}]}` — و بعد از رفع:
`{"id":…,"warehouse_id":…,"movement_type":…,"quantity":1,…}`.

**۳) یک باگ در ابزار خودم:** اسکریپت tag‌گذاری، tag را **بعد از کامنت انتهای خط**
گذاشت، پس tag داخل کامنت افتاد و بی‌اثر بود (`Quantity int // … `json:"quantity"``).
۱۱ خط اصلاح شد و یک بررسی `grep '//.*\`json:"'` به صفر رسید.

**جاروب‌های دیگر:** ۶ sentinel تعریف‌شده ولی هرگز برگردانده‌نشده (`ErrCannotPublish`,
`ErrNegativeReserved`, `ErrStorageKeyReq`, …) — بی‌خطر ولی مرده؛ و ۳ handler بدون
mount که بررسی شد: `ArticleTransitionHandler` و `transitionHandler` **builder داخلی**
هستند (نه مرده)، `GetOrderByNumberHandler` واقعاً بی‌استفاده است.

---

### ۴.۶۶ نشت PII در پیگیری سفارش — کامیت `fix(order-read-gate)`

**جدی‌ترین یافتهٔ این دوره.** `GET /orders/{orderNumber}` عمومی بود و **هیچ بررسی‌ای
نداشت**. شمارهٔ سفارش ترتیبی است (`KHA-ORD-2026-000008`)، پس هر کسی می‌توانست با شمردن
شماره‌ها سفارش‌های همه را بخواند:

```
$ curl /api/v1/orders/KHA-ORD-2026-000008
200 {"order_number":"KHA-ORD-2026-000008","guest_phone":"09120000055",…}
```

**قرارداد این را منع کرده بود** (R-07): «مهمان با phone مطابق سفارش»، پارامتر `phone`
و پاسخ `403`. پیاده‌سازی پارامتر را نادیده می‌گرفت.

**رفع — دروازهٔ خواندن:**
- سفارش مشتری: فقط با توکن همان مشتری (توکن مشتری دیگر → **۴۰۳**، بی‌توکن → **۴۰۳**)
- سفارش مهمان: `?phone=` باید با `guest_phone` برابر باشد، وگرنه **۴۰۳**
- `NormalizePhone` ارقام فارسی/عربی و پیشوند `98` را یکسان می‌کند
- پاسخ همچنان `OrderDetail` است (اقلام + fulfillment)، نه فقط سرصفحه

**شاهد:** `TestPG_GetOrderReadGate` — مالک ۲۰۰، مشتری غریبه ۴۰۳، بی‌توکن ۴۰۳، مهمان
بدون تلفن ۴۰۳، تلفن غلط ۴۰۳، تلفن درست ۲۰۰، و **ارقام فارسی** ۲۰۰.

**یک باگ جانبی که همان‌جا رفع شد:** `ShippingAddrSnap` هم tag نداشت و به‌صورت
`ShippingAddrSnap` در پاسخ می‌آمد → `shipping_address`.

**درس روشی:** `audit-routes.py` این را نمی‌دید، چون هر دو طرف را به `/orders/{}`
نرمال می‌کند و «mount شده» می‌شمارد. **هم‌شکلی مسیر، هم‌معنایی پارامتر را تضمین نمی‌کند.**

---

### ۴.۶۷ دروازهٔ تازه: انطباق قرارداد (نه فقط شکل مسیر) — کامیت `chore(audit-conformance)`

**چرا:** نشت PII در §۴.۶۶ از دید `audit-routes.py` پنهان مانده بود، چون آن ابزار فقط
**شکل** مسیر را مقایسه می‌کند (`/orders/{}` در هر دو طرف) و «mount شده» می‌شمارد. قرارداد
پارامتر `phone` و پاسخ `403` می‌خواست؛ handler هیچ‌کدام را نداشت.

**ابزار تازه `scripts/audit-contract-conformance.py`:** برای هر عملیات، پارامترهای
query اعلام‌شده در قرارداد را با کدی که واقعاً آن‌ها را می‌خواند مقایسه می‌کند (دامنهٔ
جست‌وجو: بدنهٔ handler + کل بستهٔ غیرتست، تا helperهایی مثل `mayReadOrder` حساب شوند).
در CI به‌صورت advisory اضافه شد.

**۹ واگرایی واقعی که پیدا کرد** (پارامتر اعلام‌شده، هرگز خوانده‌نشده):

```
GET /admin/audit-logs      from, to
GET /admin/orders          q
GET /catalog/products      category_slug, brand_slug, sort
GET /magazine/articles     topic_slug, q
GET /payments/return       authority
```

**این‌ها باگ خاموش‌اند:** کلاینتی که مطابق قرارداد فیلتر بفرستد، **نتیجهٔ بی‌فیلتر**
می‌گیرد و هیچ خطایی هم نمی‌بیند. نمونهٔ مشخص: کد و کلاینت خودمان هر دو `?topic=` می‌فرستند
و می‌خوانند، ولی قرارداد `topic_slug` می‌گوید — پس کلاینت بیرونی که مستندات را دنبال کند
همیشه لیست کامل را می‌گیرد.

**تصمیم مالک لازم است:** برای هر مورد، یا قرارداد اصلاح شود (v1.2.0) یا پیاده‌سازی
فیلتر اضافه شود. من عمداً هیچ‌کدام را بی‌تأیید تغییر ندادم.

**دو نکتهٔ ابزاری که خودم اصلاح کردم:** نسخهٔ اول بدنهٔ تابع را تنها می‌دید (پس
`mayReadOrder` را ندید و ۳۹ مورد false positive داد)، و بررسی کد وضعیت با جست‌وجوی
لفظی `StatusNotFound` بی‌معنا بود چون کدها مرکزی نگاشت می‌شوند — آن بررسی حذف شد.

---

### ۴.۶۸ شاخه‌های سبد — کامیت `test(cart-branches)`

`TestPG_CartServiceBranches` مسیرهای ردی که تست‌های handler نمی‌بینند:

- بدون هویت (`Cart(ctx,"","")`) → `ErrCartTokenRequired`
- تعداد ۰، منفی و ۱۱ → `ErrCartQuantityInvalid` (و ۱ تا ۱۰ مجاز)
- `variant_id` خالی → خطای کلاینت، نه panic
- **به‌روزرسانی/حذف قلم از سبد مشتری دیگر → `ErrCartItemNotFound`** (مرز مالکیت)
- `quantity: 0` در update یعنی «حذف» و سبد خالی می‌شود
- ادغام سبد مهمانِ ناموجود → no-op، نه خطا؛ ادغام بدون مشتری → خطا

**پوشش commerce: ۸۰.۴٪ → ۸۱.۱٪.**

---

### ۴.۶۹ پاک‌سازی کد مرده + گاردهای handler — کامیت `chore(cleanup-guards)`

**۶ sentinel مرده حذف شد** (تعریف‌شده، هرگز برگردانده‌نشده): `ErrCannotPublish`,
`ErrShipmentNotModifiable`, `ErrOIDCSubjectRequired`, `ErrNegativeReserved`,
`ErrStorageKeyReq`, `ErrNegativeContribution`. بررسی شد که هیچ‌کدام اعتبارسنجی گم‌شده را
علامت نمی‌دهند: media کلید ذخیره‌سازی را خودش می‌سازد (content-addressed)، inventory
وضعیت رزرو را چک می‌کند، و `ErrNegativeContribution` با BR-06 **تناقض** داشت (هشدار است،
نه خطا — R-14 خط ۴۹۲). نگه‌داشتنشان تله بود.

**`TestHandlerGuards`** — شاخه‌های زودهنگام مشترک همهٔ handlerها: بدنهٔ JSON خراب → ۴۰۰
(نه ۵۰۰)، id ناشناخته → 4xx، سبد بدون هویت → ۴۰۰ با کد `cart_token_required`، حذف قلم
ناموجود → 4xx.

**یک نکتهٔ روشی:** اولین اجرا روی حذف قلم ۵۰۰ داد. بررسی نشان داد **باگ محصول نیست**:
تست از یک UUID ساختگی برای مشتری استفاده می‌کرد و درج سبد کلید خارجی را می‌شکست. تست با
مشتری واقعی اصلاح شد. ولی خودِ آن رفتار ارزش ثبت دارد: **هویت تأییدنشده در لایهٔ سبد به
۵۰۰ می‌رسد، نه ۴۰۱** — چون id از توکن تأییدشده می‌آید، در عمل رخ نمی‌دهد، اما اگر روزی
مسیر دیگری id را از ورودی بگیرد، این یک ۵۰۰ است.

**پوشش commerce: ۸۱.۱٪ → ۸۳.۱٪.**

---

### ۴.۷۰ بستن ۹ واگرایی قرارداد — کامیت `feat(contract-filters)`

تصمیم مالک: **ترکیب** — هر جا فیلتر کاربردی است پیاده شود، هر جا پارامتر بی‌استفاده است
از قرارداد حذف شود. نتیجه: `audit-contract-conformance.py` از **۹ مشکل به clean** رسید.

**پیاده‌شده (فیلترهای واقعی):**
- **`/catalog/products`** — `category_slug` و `brand_slug` در SQL (join روی slug، بدون
  round-trip اضافه)، و `sort` با چهار حالت: `relevance` (پیش‌فرض)، `newest`،
  `price_asc`، `price_desc`. مرتب‌سازی قیمت با `LEFT JOIN LATERAL` روی ارزان‌ترین قیمت
  **تأییدشده و معتبر** هر محصول.
- **`/magazine/articles`** — `topic_slug` (حل slug → id در SQL؛ slug ناشناخته = لیست
  خالی، نه خطا) و `q` (جست‌وجو در عنوان/خلاصه، فقط PUBLISHED).
- **`/admin/orders`** — `q` روی شمارهٔ سفارش و تلفن مهمان.
- **`/admin/audit-logs`** — `from`/`to` به‌صورت بازهٔ `timestamptz`.

**حذف‌شده از قرارداد:** پارامتر `authority` در `/payments/return` (بی‌استفاده؛ مقصد
ریدایرکت از config می‌آید).

**شاهد زنده:**
```
catalog?sort=price_asc          → داده مرتب‌شده برمی‌گردد
catalog?category_slug=nope      → total 0
magazine/articles?q=خواب        → total 1
admin/orders?q=000008           → total 2
admin/audit-logs?from=2030-…    → total 0
```

**یک نشت دیگر که همان‌جا پیدا شد:** پاسخ جست‌وجوی کاتالوگ `{"Product":{…},"Snippet":…}`
برمی‌گرداند — `SearchResult` هم tag نداشت. tag اضافه شد **و کلاینت‌ها هم به‌روز شدند**
(`web/main`, `web/shop`, `web/shared/api.ts` → `hit.product`)، چون این کلیدها باربری
داشتند. بیلد هر دو اپ سبز.

**پوشش SQL: ۹۲/۹۲** (سه متد تازه: `ListArticlesByTopicSlug`, `SearchPublishedArticles`,
و پارامترهای تازهٔ `List`).

---

### ۴.۷۱ پنل انبار — تست کامل — کامیت `test(inventory-admin)`

کل `admin.go` موجودی **۰٪** بود: `ListStock`, `ListReservations`, `RecordMovementHandler`
و کمکی‌های صفحه‌بندی. `TestPG_InventoryAdminHandlers` همه را می‌راند:

- **فهرست موجودی** — ردیف ساخته‌شده با `on_hand_quantity=12` دیده می‌شود؛ فیلتر `q` روی
  SKU یک ردیف می‌دهد و برای SKU ناموجود صفر؛ بلوک `page` (`page`/`total`) در پاسخ هست.
- **فهرست رزروها** — خالی ۲۰۰؛ `expires_before` خراب → **۴۰۰**؛ بعد از رزرو، ردیف در
  فهرست دیده می‌شود.
- **حرکت دستی** — نوع `DAMAGE` با مقدار منفی پذیرفته می‌شود؛ نوع بی‌معنا → 4xx؛ فیلد
  ناقص → **۴۰۰**.

**پوشش inventory: ۶۲.۸٪ → ۷۸.۳٪.**

---

### ۴.۷۲ سه باگ در قیمت‌گذاری — کامیت `fix(pricing-branches)`

تست شاخه‌های خطا سه نقص واقعی داد:

**۱) بدنهٔ خراب → ۵۰۰، نه ۴۰۰.** هر دو handler نوشتن (`cost`, `price`) خطای JSON را به
`writePricingErr` می‌دادند که برای آن خطا شاخه‌ای ندارد → `default` → ۵۰۰. حالا مستقیم
۴۰۰ با کد `invalid_body`.

**۲) `target_contribution_rate` خارج از بازه پذیرفته می‌شد.** قرارداد
`minimum: 0, maximum: 1` می‌گوید و `ErrInvalidTargetRate` هم وجود داشت، ولی **هیچ‌جا
return نمی‌شد** — نرخ ۱.۵ ذخیره می‌شد. اعتبارسنجی در `CreatePriceRecord` وصل شد.

**۳) خطای audit بلعیده می‌شد.** `svc.audit` نتیجه را `_ =` می‌کرد، پس رکورد بدون هیچ
ردی در تریل ذخیره می‌شد و کسی نمی‌فهمید. حالا خطا برمی‌گردد و handler آن را به‌صورت
`audit_warning` در پاسخ **۲۰۱** می‌گذارد — نه ۵xx (چون رکورد ذخیره شده و retry رکورد
تکراری می‌سازد) و نه سکوت.

**یک فرض غلط من:** انتظار داشتم unit economics برای variant ناشناخته ۴۰۴ بدهد؛ طراحی
درست این است که **۲۰۰ با warnings** بدهد (می‌گوید چه چیزی کم است). تست با رفتار مستند
هم‌تراز شد.

**پوشش pricing: ۷۹.۹٪ → ۸۶.۲٪.**

---

### ۴.۷۳ جاروب کلاس‌ها با اسکریپت — کامیت `fix(error-classes)`

به‌جای ماژول‌به‌ماژول، این بار **کلاس‌های باگ** را در همهٔ کد جارو کردم. سه جاروب:

**الف) sentinelهایی که به کد وضعیت نگاشت شده‌اند ولی هرگز return نمی‌شوند** (اعتبارسنجی
مرده): `catalog.ErrCategoryNotFound`, `catalog.ErrVariantNotFound`,
`commerce.ErrRefundNotFound`, `commerce.ErrInvalidTransition`,
`commerce.ErrLineTotalsMismatch`, `content.ErrDuplicateSlug`,
`fulfillment.ErrAllocationExceedsOrder`, `inventory.ErrWarehouseNotFound`.
این‌ها ۴۰۴/۴۰۹ وعده می‌دهند که هیچ‌وقت صادر نمی‌شود — همان الگویی که
`ErrInvalidTargetRate` را در قیمت‌گذاری لو داد.

**ب) handlerهایی که خطای کلاینت را به ۵۰۰ تبدیل می‌کنند:** پنج handler نوشتن کاتالوگ
(`create/update product`, `create category`, `create variant`, `product status`) خطای JSON
را به `writeCatalogErr(errors.New(...))` می‌دادند که `default` آن ۵۰۰ است. **رفع:** مستقیم
۴۰۰ با کد `invalid_body`.

**ج) audit بلعیده‌شده:** `catalog.Service.audit` و `commerce.CreateRefund` نتیجهٔ
`Record` را دور می‌ریختند. حالا خطا برمی‌گردد؛ در کاتالوگ به‌صورت `audit_warning` در بدنه
(یا هدر `X-Audit-Warning` برای پاسخ منبع) و در بازپرداخت به‌صورت خطای صریح
«refund issued but audit failed» — چون پول جابه‌جا شده و سکوت بدترین گزینه است.

**شاهد:** `TestAdminHandlersRejectBadJSON` (پنج handler، بدنهٔ خراب → ۴۰۰) و
`TestAuditFailureIsVisible` (خطای audit برمی‌گردد؛ auditor تهی no-op است).

**پوشش catalog: ۵۹.۳٪ → ۶۳.۴٪.**

---

### ۴.۷۴ اعتبارسنجی‌های مرده: دو وصل شد، پنج حذف — کامیت `fix(dead-validations)`

هشت sentinel که به کد وضعیت نگاشت شده بودند ولی هرگز صادر نمی‌شدند بررسی شدند. **دو
مورد باگ واقعی بودند** (خطای کلاینت → ۵۰۰)، بقیه واقعاً مرده.

**وصل شد (باگ واقعی):**
- **`content.ErrDuplicateSlug`** — slug تکراری به ایندکس یکتا می‌خورد و **۵۰۰** می‌داد.
  حالا `isUniqueViolation` (SQLSTATE 23505) آن را به `ErrDuplicateSlug` → **۴۰۹** نگاشت
  می‌کند.
- **`inventory.ErrWarehouseNotFound`** — رسید/تعدیل روی انبار ناموجود کلید خارجی را
  می‌شکست و **۵۰۰** می‌داد. حالا `ReceiveStock` و `AdjustStock` وجود انبار را چک می‌کنند
  → **۴۰۴**.

**حذف شد (مرده):** `catalog.ErrVariantNotFound`، `commerce.ErrRefundNotFound`،
`commerce.ErrInvalidTransition`، `commerce.ErrLineTotalsMismatch` — و ورودی‌هایشان در
نگاشت‌ها. دو مورد جاروب قبلی هم false positive بودند: `ErrAllocationExceedsOrder` داخل
`fmt.Errorf(%w)` برمی‌گردد و `ErrCategoryNotFound` مستقیم به `writeCatalogErr` داده می‌شود.

**شاهد:** `TestPG_DuplicateSlugIsAConflict` (تکراری → `ErrDuplicateSlug`) و
`TestPG_UnknownWarehouseIsNotFound` (رسید و تعدیل روی انبار ناموجود → `ErrWarehouseNotFound`).

**یک تست موجود شکست و درست بود:** `TestAdjustStockWritesAudit` انبار نمی‌ساخت و حالا
`ErrWarehouseNotFound` می‌گرفت — یعنی اعتبارسنجی تازه کار می‌کند. تست اصلاح شد.

**سنتینل مرده باقی‌مانده: صفر.**

---

### ۴.۷۵ pricing به آستانه رسید + گیت پوشش — کامیت `test(pricing-threshold)`

**اولین ماژول مالی که آستانهٔ R-CI §۳ را برآورده کرد: pricing ۸۶.۲٪ → ۹۰.۰٪.**

دو تست تازه:
- `TestPG_PricingReadPaths` — فهرست‌های هزینه/قیمت، قیمت ویترین (۲۰۰ برای قیمت تأییدشده،
  **۴۰۴** برای واریانت بی‌قیمت)، و economics عمومی.
- `TestPricingStoreFailuresAreServerErrors` — با یک `brokenStore` (بدون دیتابیس) همهٔ
  مسیرهای خطای handlerها: **خرابی store باید ۵xx بدهد، نه ۲۰۰ با لیست خالی.**
- `TestDiscountAmount` — سیاست خالی/خراب/منفی → صفر.

**گیت تازه `scripts/audit-coverage.py`:** آستانه‌ها از R-CI §۳ (۹۰٪ مالی/موجودی، ۷۵٪ سایر)
با یک مجموعهٔ **ENFORCED** صریح. فقط `pricing` اکنون blocking است؛ بقیه advisory.

**چرا این‌طور:** اجرای همگانی آستانه یعنی یا پایین آوردن عدد یا بلاک کردن CI روی کاری که
نشده — هر دو ناصادقانه. هر ماژول وقتی به آستانه رسید به مجموعهٔ ENFORCED اضافه می‌شود.

**وضعیت فعلی (اندازه‌گیری‌شده):**
```
pricing         90.0% / 90%   ok  [ENFORCED]
commerce        83.0% / 90%   below [advisory]
inventory       78.4% / 90%   below [advisory]
authorization   76.1% / 75%   ok
worker          86.5% / 75%   ok
notifications   83.3% / 75%   ok
catalog 63.4 · suppliers 60.2 · fulfillment 60.8 · identity 51.2 · media 47.4
content 46.8 · ecosystem 40.0 · audit 0.0 · shared 0.0   (advisory)
```

---

### ۴.۷۶ دو باگ در سبد و بازپرداخت — کامیت `fix(cart-expiry-refund-guard)`

**۱) سبد منقضی هنوز ACTIVE برگردانده می‌شد.** `FindCartBySession` و `GetOrCreateCart` هر
دو `expires_at` را نادیده می‌گرفتند، پس سبد ۳۰ روزهٔ منقضی همچنان استفاده و **ادغام**
می‌شد — یعنی اقلام کهنه به سبد مشتری می‌رفتند. هر دو کوئری حالا
`(expires_at IS NULL OR expires_at > now())` دارند.

**۲) `Service.ListRefunds` روی `s.refunds` تهی panic می‌کرد.** کامنت فیلد می‌گفت
«nil → refund methods error»، ولی سه متد بدون گارد dereference می‌کردند. گارد
`ErrRefundStoreNotWired` به `ListRefunds`، `CreateRefund` و `ReconcileTransaction` اضافه شد.

**تست‌ها:**
- `TestPG_MergeCartsEdgeCases` — ادغام با واریانت مشترک (جمع تعداد: ۱+۲=۳)، واریانت تازه،
  ادغام سبد در خودش (no-op)، سبد مهمان خالی، **سبد منقضی** (no-op)، و ادغام بدون مشتری.
- `TestPG_CommerceStoreFailuresAreServerErrors` — با `failingStore` (embedding + override)
  شاخه‌های خطای `ListRefunds`, `Reconcile`, `GetOrder`, `CreateRefund`: خرابی store باید
  ۵xx بدهد، نه ۲۰۰ با لیست خالی.

**پوشش commerce: ۸۳.۰٪ → ۸۳.۶٪.**

---

### ۴.۷۷ باگ ۵۰۰ در `/me` + تست جریان OTP — کامیت `fix(me-unauthorized)`

**باگ:** `MeHandler` (و چهار handler آدرس) هویت را از context می‌خواندند و **بدون بررسی**
به store می‌دادند. با هویت خالی، `''::uuid` در کوئری خطا می‌دهد → **۵۰۰ به‌جای ۴۰۱**.

در عمل مسیر پشت `AuthenticateCustomer` است، پس رخ نمی‌داد — ولی همان الگوی «مسیر دفاعی
ندارد» است که در inventory و سبد هم باگ داد. گارد `requireCustomer` به هر پنج handler
اضافه شد: بدون هویت → **۴۰۱** با کد `unauthorized`.

**تست تازه `TestPG_OTPFlowThroughHandlers`** — کل سطح احراز هویت که تا حالا فقط دستی
آزموده شده بود:
- بدنهٔ خراب و شمارهٔ غایب → ۴۰۰
- `OTPStart` کد را «تحویل» می‌دهد (کال‌بک)
- کد غلط → 4xx؛ کد درست → **۲۰۰** با توکن و `service: shop`
- توکن صادرشده با `DBSubjectResolver` حل می‌شود
- `/me` با هویت → ۲۰۰، **بدون هویت → ۴۰۱** (قبل از رفع: ۵۰۰)

**پوشش identity: ۵۱.۲٪ → ۵۷.۵٪.**

---

### ۴.۷۸ ورود کارمند و ماتریس RBAC — کامیت `test(staff-login)`

`TestPG_StaffLoginAndAdminReads` سطح پنل را می‌راند:

- بدنهٔ خراب و گذرواژهٔ غایب → ۴۰۰
- گذرواژهٔ غلط → ۴۰۱، **ایمیل ناشناخته هم دقیقاً ۴۰۱** (بدون شمارش کاربر: bcrypt روی
  هش ساختگی اجرا می‌شود)
- حساب **SUSPENDED** با گذرواژهٔ درست هم رد می‌شود
- ورود موفق → توکنی که `DBSubjectResolver` حلش می‌کند
- `GET /admin/users` (شامل حساب تازه)، `/admin/roles`، `/admin/permissions` → ۲۰۰

**این بار باگی پیدا نشد** — ورود کارمند درست پیاده شده بود (bcrypt cost 12، بدون
user enumeration، گارد وضعیت).

**پوشش identity: ۵۷.۵٪ → ۶۳.۵٪.**

**یافتهٔ امنیتی که ثبت می‌شود ولی بی‌تأیید تغییر نکرد:** `AdminLogin` **محدودیت نرخ
ندارد**. OTP سقف ۵ در ساعت دارد، ولی گذرواژهٔ کارمند را می‌شود بی‌نهایت حدس زد. افزودن
قفل پس از n تلاش یک **سیاست** است (چه n؟ قفل موقت یا دائمی؟ بر اساس ایمیل یا IP؟) و طبق
قاعدهٔ مالک باید تصمیم گرفته شود، نه سلیقه‌ای اعمال.

---

### ۴.۷۹ تست HTTP محتوا و رسانه + نشت PascalCase در media — کامیت `test(content-media-http)`

`content/http.go` (۲۰۲ گزاره) و `media/http.go` (۶۵ گزاره) **صفر درصد** بودند.

**`TestPG_ContentHTTPHandlers`** — مسیر تحریریه از طریق handlerها:
- ساخت موضوع و مقاله؛ بدنهٔ خراب → **۴۰۰** (نه ۵۰۰)
- **slug تکراری → ۴۰۹**
- چرخهٔ submit → approve → publish؛ و **انتشار بدون منبع رد می‌شود** (دروازهٔ
  ADR-CONTENT-001 واقعاً کار می‌کند)، با منبع → ۲۰۰
- خواندن‌های ویترین: موضوعات، مقالات منتشرشده، مقاله با slug؛ slug ناشناخته → ۴۰۴

**`TestPG_MediaHTTPHandlers`** — آپلود multipart، متادیتا، دانلود بایت‌ها، حذف:
- فیلد فایل غایب → ۴۰۰، نوع رسانهٔ نامعتبر → 4xx
- آپلود واقعی → ۲۰۱ با `storage_key`؛ متادیتا ۲۰۰؛ محتوا همان بایت‌ها؛ حذف ۲۰۴ و
  **حذف دوم 4xx**

**باگ پیدا شده: نشت PascalCase در `Asset`.** پاسخ آپلود
`{"ID":…,"MediaType":…,"StorageKey":…}` بود. tagها اضافه شد →
`{"id":…,"media_type":…,"storage_key":…}`. کلاینت وب این کلیدها را نمی‌خواند، پس
شکستن رخ نداد.

**یک سردرگمی که رفع شد:** اولین اجرا `relation "media_assets" does not exist` داد.
بررسی نشان داد `testpg.Setup` فقط schema می‌سازد و migration را اعمال نمی‌کند — تست
باید خودش `migrations.ApplyAll` را صدا بزند (مثل بقیهٔ بسته‌ها). باگ محصول نبود.

**پوشش: content ۴۶.۸٪ → ۶۰.۷٪ · media ۴۷.۴٪ → ۷۹.۸٪.**

---

### ۴.۸۰ ecosystem و fulfillment — کامیت `test(eco-ful-http)`

**ecosystem ۴۰٪ → ۹۷.۸٪** (`TestSearchHandler`, `TestNavigationHandler`):
- پرس‌وجوی خالی → **۴۰۰** (نه دامپ کل کاتالوگ)
- پرس‌وجو به هر دو منبع می‌رسد؛ نتایج و شمارش‌ها درست
- صفحه‌بندی بی‌معنا → پیش‌فرض، نه خطا
- **خرابی یک منبع → ۵۰۰**، نه ۲۰۰ نیمه‌خالی
- ناوبری مشترک غیرخالی

**fulfillment ۶۰.۸٪ → ۷۵.۰٪** (`TestPG_FulfillmentHTTPHandlers`):
- بدنهٔ خراب → ۴۰۰؛ **`order_id` غایب → ۴۰۰** (قبل از رفع: ۵۰۰)
- چرخهٔ کامل PENDING → CONFIRMED → PROCESSING → READY، سپس ساخت محموله،
  IN_TRANSIT، و افزودن رخداد رهگیری
- انتقال معکوس → 4xx؛ id ناشناخته → 4xx؛ منبع رخداد نامعتبر → 4xx

**باگ پیدا شده:** `ErrOrderIDRequired` و `ErrInvalidEventSource` در `writeFulSvcErr`
نگاشت نشده بودند، پس خطای کلاینت ۵۰۰ می‌گرفت — همان کلاس تکراری.

**دو باگ در تست خودم که ارزش ثبت دارند:**
1. حلقه روی `map[string]http.HandlerFunc` — ترتیب تصادفی Go باعث شد `processing` قبل از
   `confirm` اجرا شود و ۴۰۹ بگیرد. با slice مرتب جایگزین شد.
2. handler انتقال، فیلد `to` را می‌خواند نه `status`؛ تست `status` می‌فرستاد. **این یک
   ناسازگاری واقعی است** که باید بررسی شود: قرارداد برای بدنهٔ این مسیر چه نامی می‌گوید؟

---

### ۴.۸۱ جهت معکوس قرارداد: ۶۸ مسیر مستندنشده — کامیت `chore(route-audit-reverse)`

**یافتهٔ تازه:** تا حالا فقط یک جهت را می‌سنجیدم — «مسیر قراردادی که mount نشده».
جهت معکوس هرگز اندازه‌گیری نشده بود:

```
mounted: 126 | declared: 58
MOUNTED BUT NOT IN THE CONTRACT: 68
```

**۶۸ مسیر کار می‌کند ولی در قرارداد نیست** — یعنی کلاینت بیرونی نمی‌تواند کشفش کند:

- **۶ مورد alias** با شکل قراردادی موجود: `GET /articles`, `/categories`, `/search`,
  `/topics`, `/navigation`, `POST /orders` (پیشوند متفاوت، همان قابلیت)
- **۶۲ مورد بدون هیچ معادلی در قرارداد:** کل `media` (آپلود/متادیتا/محتوا/حذف)،
  خواندن‌های پنل (`/admin/users`, `/admin/products`, `/admin/orders/{}/refunds`,
  `/admin/variants/{}/economics`, `/admin/variants/{}/cost-records`,
  `/admin/warehouses/{}/variants/{}/movements`, …)، و `/shipments/{}/transition`

**بستن این شکاف یعنی بازبینی قرارداد (v1.2.0)** و طبق قاعدهٔ مالک نیاز به تأیید دارد.
پس فقط ابزار را صریح کردم:

- `--fail` = فقط «قراردادی که mount نشده» (وعدهٔ سخت؛ اکنون **۰** و سبز)
- `--fail-undocumented` = جهت معکوس (opt-in؛ اکنون **۶۸** و قرمز)

**اثبات رفتار گیت:**
```
audit-routes.py --fail               → exit 0   (هیچ وعدهٔ شکسته‌ای نیست)
audit-routes.py --fail-undocumented  → exit 1   (۶۸ مسیر مستندنشده)
```

**تصمیم لازم:** آیا ۶۲ مسیر مستند شوند (بازبینی قرارداد) یا بخشی حذف/ادغام شود؟ تا آن
زمان گیت معکوس خاموش می‌ماند ولی **قابل روشن شدن با یک فلگ** است.

---

### ۴.۸۲ مستندسازی ۶۸ مسیر + گیت دوطرفه — کامیت `docs(contract-full)`

تصمیم مالک: **گزینهٔ ۲** — مستندسازی مسیرهای کاربردی.

**قرارداد از ۵۸ به ۱۲۱ مسیر رسید.** همهٔ ۶۸ مسیر مستندنشده اضافه شدند: کل `media`،
خواندن‌های پنل (`/admin/users`, `/admin/products`, `/admin/orders/{}/refunds`,
`/admin/variants/{}/economics|cost-records|price-records`,
`/admin/warehouses/{}/variants/{}/movements`, ارزیابی تأمین‌کننده، تاریخچهٔ قیمت خرید،
رزروهای موجودی)، `shipments/{}/transition|events`، و aliasهای ویترین.

**پنج مسیر تکراری حذف شد و کد با قرارداد هم‌نام شد.** معلوم شد
`/admin/products/{id}` در کد با `/admin/products/{productId}` در قرارداد **همان مسیر**
است. کد تغییر کرد (Go نام پارامتر را نمی‌بیند) و ورودی تکراری از قرارداد حذف شد — به‌جای
داشتن دو نام برای یک چیز.

**هشت tag تازه** به فهرست global اضافه شد تا هشدار `operation-tag-defined` صفر شود.

**شاهد:**
```
spectral:            0 errors, 4 warnings   (فقط چهار کامپوننت بی‌استفاده)
audit-routes --fail:                 exit 0   (هیچ وعدهٔ شکسته)
audit-routes --fail-undocumented:    exit 0   (هیچ مسیر مستندنشده)
mounted: 126 | declared: 121
```

**گیت CI سخت شد:** `audit-routes.py --fail --fail-undocumented` از advisory به **blocking**
تغییر کرد — هر دو جهت قرارداد اکنون CI را می‌شکنند.

---

### ۴.۸۳ suppliers — کامیت `test(suppliers-http)`

`suppliers/http.go` **۱۰۳ گزاره، ۰٪**.

`TestPG_SuppliersHTTPHandlers` — پنل تأمین‌کننده از طریق handlerها:
- بدنهٔ خراب و نام غایب → ۴۰۰
- ساخت → ۲۰۱؛ ویرایش → ۲۰۰ و **نام در پایگاه داده واقعاً عوض شد**
- ارزیابی با امتیاز بیرون از بازه → 4xx؛ ارزیابی معتبر → ۲۰۱ و در فهرست دیده می‌شود
- تأمین‌کنندهٔ ناشناخته: فهرست ارزیابی/تاریخچهٔ قیمت/واریانت‌ها → 2xx خالی، نه ۵۰۰

**باگ پیدا شده — ۴۰۹ برای ورودی نامعتبر:** وضعیت `NONSENSE` پاسخ **۴۰۹ Conflict**
می‌گرفت. علت: یک sentinel برای دو چیز متفاوت:

```go
if newStatus != ACTIVE && ... { return ErrInvalidStatus }   // مقدار ناموجود
if !allowedTransition(...)    { return ErrInvalidStatus }   // تعارض واقعی وضعیت
```

۴۰۹ یعنی «تعارض با وضعیت فعلی منبع»؛ مقدار ناموجود خطای ورودی است. `ErrUnknownStatus`
اضافه شد → **۴۰۰**، و ۴۰۹ فقط برای انتقال غیرمجاز بین دو وضعیت معتبر ماند.

**دو تست قدیمی همین sentinel اشتباه را تثبیت کرده بودند** (`"NOPE"` → `ErrInvalidStatus`)
— یعنی تست سبز بود ولی قرارداد HTTP غلط. هر دو اصلاح شدند.

**پوشش: suppliers ۶۰.۲٪ → ۷۸.۵٪** (آستانهٔ ۷۵٪ برآورده شد).

---

### ۴.۸۴ catalog — کامیت `test(catalog-http)`

`admin.go` ۲۷٪ و `http.go` ۴۸٪ بودند.

`TestCatalogHTTPHandlers` — ویترین و پنل روی memStore:
- بدنهٔ خراب در ساخت دسته/محصول/واریانت/ویرایش → ۴۰۰
- **دروازهٔ انتشار (MA-1/MA-6) واقعاً کار می‌کند:** محصول بدون
  `sleep_relevance`/`spec_source`/`spec_last_reviewed_at` → **۴۲۲** با کد
  `publish_gate_missing`؛ با آن‌ها → انتشار موفق
- **قاعدهٔ دیده‌شدن ویترین:** DRAFT در ویترین ۴۰۴ ولی در پنل ۲۰۰؛ پس از انتشار ۲۰۰؛
  پس از PAUSED دوباره ۴۰۴
- وضعیت `NONSENSE` → 4xx؛ id ناشناخته → ۴۰۴ (نه ۵۰۰)؛ SKU تکراری → 4xx
- صفحه‌بندی بی‌معنا → پیش‌فرض

**سه فرض غلط من که قبل از ادعای باگ بررسی شدند:**
1. `PATCH /admin/products/{id}` با فقط `title` → ۵۰۰. علت: `s.updates` در تست سیم‌کشی
   نشده بود (`SetUpdateStore`). **باگ محصول نبود.**
2. `CreateVariantHandler` با `{id}` در مسیر → ۴۰۴. علت: مسیر واقعی
   `POST /admin/variants` است و `product_id` از **بدنه** می‌آید. **باگ نبود.**
3. `spec_last_reviewed_at` با `"2026-01-01"` → ۴۰۰. علت: نوع `*time.Time` و نیاز به
   RFC3339. **باگ نبود.**

**پوشش: catalog ۶۳.۴٪ → ۷۱.۰٪** (هنوز زیر ۷۵٪؛ `admin.go` ۲۷٪ → و `catalog_db.go` ۷۸٪).

---

### ۴.۸۵ catalog به آستانه رسید — کامیت `test(catalog-threshold)`

**`TestPG_AdminProductList`** — `NewAdminListStore`, `List`, `ListProductsHandler` هر سه
**۰٪** بودند:
- فیلتر `status` (PUBLISHED → ۰، DRAFT → هر دو)
- فیلتر `q` روی title و slug
- **پرچم `publish_gate_satisfied` در هر ردیف** — محصول کامل `true`، ناقص `false`
- `page_size=5000` → سقف **۱۰۰**؛ `page=abc` → پیش‌فرض ۱/۲۰

**`TestPG_UpdateProductPartialPatch`** — `DBStore.UpdateProduct` **۰٪** بود:
- patch فقط `title` → slug و توضیح دست‌نخورده می‌مانند
- جایگزینی مجموعهٔ دسته‌بندی درست کار می‌کند
- patch خالی no-op است، نه خطا
- id ناشناخته → `ErrProductNotFound` (نه خطای درایور)
- **slug تکراری رد می‌شود** (تعارض قابل نگاشت، نه ۵۰۰ خام)

**پوشش: catalog ۶۳.۴٪ → ۷۱.۰٪ → ۷۷.۲٪** — آستانهٔ ۷۵٪ برآورده شد.

---

### ۴.۸۶ shared — کامیت `test(shared-actor)`

`shared` صفر درصد بود، ولی `ActorID` مسیر مشترک **همهٔ** handlerهای حساس است
(سه باگ «هویت از بدنه» از همین‌جا می‌آمد).

**باگ پیدا شده:** `ActorID(nil)` **panic** می‌کرد (nil pointer dereference روی
`ctx.Value`). قرارداد تابع می‌گوید «برای فراخوانی ناشناس `""` برمی‌گرداند» — و nil
حداکثر حالت ناشناس است. دو خط `if ctx == nil { return "" }` اضافه شد: فراخواننده‌ای که
context را از دست داده باید **۴۰۱** بگیرد، نه اینکه فرآیند را بیندازد.

**`TestActorIDContract`** — ۱۰۰٪ پوشش بسته:
- ناشناس → `""`؛ nil → `""` (قبلاً panic)
- رفت‌وبرگشت مقدار
- context مشتق‌شده مقدار را به ارث می‌برد؛ override در فرزند، **والد را تغییر نمی‌دهد**
- دو context خواهر به هم نشت نمی‌کنند
- مقدار غیررشته‌ای در slot → `""` نه panic (type assertion با comma-ok)
- id خالی → `""`

**پوشش: shared ۰٪ → ۱۰۰٪.**

---

### ۴.۸۷ audit — کامیت `test(audit-trail)`

`audit` **۰٪** بود — و این خطرناک‌ترین شکاف پوششی پروژه: IAM §۷ تریل را برای عملیات مالی
الزامی کرده، و recorder‌ای که بی‌صدا هیچ نمی‌نویسد هیچ تستی را نمی‌شکند.

**`TestPG_RecorderWritesTheTrail`:**
- رکورد کامل: actor، action، entity، before/after و ip — همه از پایگاه داده بازخوانی شدند
- **رشته‌های خالی به SQL NULL تبدیل می‌شوند، نه `""`** — ردیف تریل نباید مدعی actorی باشد
  که وجود ندارد
- مقدار غیرقابل marshal (`chan`) → خطا **و هیچ ردیفی نوشته نمی‌شود** (نه رکورد نیمه)

**`TestPG_OutboxPublish`** (ADR-ARCH-007):
- ردیف با `published_at = NULL` می‌نشیند تا worker بردارد
- payload رفت‌وبرگشت می‌کند
- payload `nil` → ستون NULL، نه رشتهٔ `"null"`

**`TestPG_AuditList`:**
- فیلتر `action` و `actor_id`
- پنجرهٔ `from`/`to`: آینده → ۰، گذشته → همه
- صفحه‌بندی و سقف

**سه نکتهٔ schema که فقط با اجرا معلوم شد:**
- `actor_id`، `entity_id` و `aggregate_id` ستون **uuid** هستند — مقدار غیر-UUID خطای
  درایور می‌دهد، نه ۵۰۰
- `ip_address` ستون **inet** است و برای اسکن به `::text` نیاز دارد؛ و آدرس خالی به
  `10.0.0.1/32` نرمال می‌شود

**پوشش: audit ۰٪ → ۸۵.۲٪.**

---

### ۴.۸۸ content — کامیت `test(content-edges)`

**`TestPG_ContentServiceEdges`** — شاخه‌های سرویس که تست HTTP به آن‌ها نمی‌رسید:
- **گاردهای id خالی** → sentinel، نه کوئری (`GetArticle`, `ListTranslations`,
  `DeleteArticleSource`, `ListRelationsForArticle`)
- **ترجمه بدون `parent_article_id` رد می‌شود** (SAD §۷.۲)؛ با آن → ساخته می‌شود و در
  زنجیرهٔ ترجمه دیده می‌شود
- نوع منبع نامعتبر → `ErrInvalidArticleSource`
- منبع مقاله: افزودن، فهرست، حذف، و **پس از حذف خالی است**
- **رابطهٔ محصول بدون `relation_reason` رد می‌شود** (MA-5)؛ نوع نامعتبر و idهای غایب هم
- پیوند موضوع و فهرست موضوعات مقاله
- **مقالهٔ DRAFT در فهرست منتشرشده‌ها نیست**

**`TestPG_ArticleLifecycleAndSearch`:**
- submit → approve → publish، سپس **unpublish واقعاً وضعیت را عوض می‌کند**، سپس archive
- id ناشناخته در انتقال → 4xx
- `UnlinkArticleTopic` و `RemoveProductArticleRelation` (هر دو ۰٪)
- **جست‌وجو فقط مقالهٔ منتشرشده را می‌بیند**؛ عبارت بی‌معنا → ۰

**دو باگ در تست خودم:**
1. **دوباره حلقه روی `map[string]http.HandlerFunc`** — همان اشتباهی که در fulfillment
   وقت گرفت؛ `approve` قبل از `submit` اجرا شد و ۴۰۹ داد. با slice مرتب جایگزین شد.
2. مقاله‌ای که به SCHEDULED رفته بود را برای مسیر جست‌وجو استفاده کردم؛ جست‌وجو فقط
   PUBLISHED را می‌بیند. مقالهٔ جداگانه ساخته شد.

**پوشش: content ۶۰.۷٪ → ۶۶.۷٪** — **آستانهٔ ۷۵٪ هنوز برآورده نشده**؛ `http.go` با
۱۱۹ گزارهٔ پوشش‌نشده بزرگ‌ترین شکاف باقی‌مانده است.

---

### ۴.۸۹ content به آستانه رسید — کامیت `test(content-http-edges)`

`TestPG_ContentHTTPEdges` — هفت handler که ۰٪ بودند:
- **منابع:** بدنهٔ خراب → ۴۰۰؛ افزودن → ۲۰۱؛ فهرست شامل نقل‌قول؛ مقالهٔ ناشناخته → لیست
  خالی نه ۵۰۰
- **موضوع:** بدنهٔ خراب → ۴۰۰؛ پیوند → ۲۰۰ و در فهرست موضوعات مقاله دیده می‌شود
- **رابطهٔ محصول:** بدنهٔ خراب → ۴۰۰؛ **بدون `relation_reason` → 4xx** (MA-5 در لایهٔ HTTP
  هم اعمال می‌شود)
- **ویرایش:** بدنهٔ خراب → ۴۰۰؛ `title` تنها → ۲۰۰ و **`body` دست‌نخورده می‌ماند** و مقدار
  در پایگاه داده واقعاً عوض شد
- **تصمیم بازبینی:** بدنهٔ خراب → ۴۰۰؛ `NONSENSE` → 4xx؛ `submit` → موفق؛ `reject` →
  وضعیت به DRAFT برمی‌گردد
- **خواندن محصول‌محور:** مقالات منتشرشدهٔ یک محصول → ۲۰۰

**پوشش: content ۶۰.۷٪ → ۶۶.۷٪ → ۷۸.۹٪** — آستانهٔ ۷۵٪ برآورده شد.

---

### ۴.۹۰ identity به آستانه رسید — آخرین ماژول — کامیت `test(identity-threshold)`

**`TestPG_CustomerHTTPHandlers`** — `customer_http.go` ۱۸٪:
- **همهٔ پنج مسیر `/me` بدون احراز هویت → ۴۰۱**
- پروفایل: `phone` و `phone_verified` از پایگاه داده
- دفتر آدرس: بدنهٔ خراب → ۴۰۰، ساخت → ۲۰۱، فهرست، ویرایش، حذف → ۲۰۴، **حذف دوم ۴۰۴**

**باگ پیدا شده — ترتیب احراز هویت و ورودی:** `CreateAddressHandler` و
`UpdateAddressHandler` **قبل از** `requireCustomer` بدنه را decode می‌کردند. نتیجه: فراخوانی
بدون احراز هویت با بدنهٔ خراب **۴۰۰** می‌گرفت، نه **۴۰۱**. احراز هویت باید اول باشد —
کاربری که اجازهٔ فرستادن بدنه را ندارد نباید بازخورد اعتبارسنجی بگیرد. ترتیب در هر دو
handler اصلاح شد.

**واگرایی ثبت‌شده (تصمیم مالک لازم):** مسیر `PATCH /me/addresses/{id}` است ولی handler
**آدرس کامل** می‌خواهد (`province`, `city`, `address_line`) — یعنی مثل `PUT` رفتار می‌کند.
تست رفتار فعلی را تثبیت کرد تا واگرایی دیده شود؛ تبدیلش به partial update تصمیم قراردادی است.

**`TestPG_AdminUserHandlers`** — `adminusers.go` ۴۶٪:
- کف گذرواژه ۱۲ کاراکتر با کد `weak_password`
- ساخت واقعی با نقش؛ **نقش در `admin_user_roles` نشست** (از پایگاه داده تأیید شد)
- ایمیل تکراری → 4xx، نه ۵۰۰
- ویرایش نام و وضعیت → ۲۰۰ و مقدار واقعاً عوض شد؛ id ناشناخته → ۴۰۴

**`TestPG_OTPLoginPage`** — `otplogin.go` ۰٪:
- بدون `continuation` → ۴۰۰ (فرم مرده رندر نمی‌شود)
- صفحه: `text/html`، `Cache-Control: no-store`، توکن در صفحه
- **تزریق markup خنثی می‌شود** — `"><script>alert(1)</script>` به‌صورت literal JS
  (`\u003cscript`) رندر می‌شود، نه HTML

**`TestPG_OTPLoginSubmit`** — هر سه فیلد الزامی؛ کد غلط → 4xx نه ۵۰۰ و بدون افشای وجود شماره

**`TestSessionCookieFlags`** — `sessioncookie.go` ۶۴٪:
- **`HttpOnly` همیشه**؛ `Secure` روی TLS و **پشت پروکسی با `X-Forwarded-Proto: https`**
  (بدون این، کوکی در production بدون `Secure` صادر می‌شد)
- `Path=/oauth` — کوکی از سطح API کارمند دور می‌ماند
- `SameSite=Lax`، `Max-Age` درست
- logout → ۲۰۴ و کوکی پاک می‌شود با همان path

**پوشش: identity ۶۳.۵٪ → ۷۲.۰٪ → ۷۴.۹٪ → ۷۵.۴٪** — آستانه برآورده شد.

**۱۵ از ۱۵ بسته حالا بالای آستانه‌اند.**

---

### ۴.۹۱ گیت conformance واقعاً بسته شد — کامیت `chore(ci-conformance-gate)`

**گیت conformance دو بار بی‌اثر بود:**

```yaml
continue-on-error: true                                  # ۱) خطا نادیده گرفته می‌شد
run: python3 scripts/audit-contract-conformance.py       # ۲) و --fail هم پاس نمی‌شد
```

اسکریپت از قبل `--fail` داشت و `sys.exit(1)` می‌کرد، ولی CI بدون آن صدایش می‌زد. یعنی
حتی اگر `continue-on-error` هم برداشته می‌شد، **گیت هرگز نمی‌گرفت**.

**اثبات پس از اصلاح** — یک واگرایی واقعی تزریق شد (خواندن `q` از
`GET /admin/orders` حذف شد):

```
audit-contract-conformance.py --fail   → exit 1
  GET /admin/orders   query parameter "q" is declared but never read
پس از بازگردانی                        → exit 0
```

**`golangci-lint` نتوانستم تأیید کنم — و این را صریح می‌گویم.** اجرای محلی با
`go run ...@latest` شکست خورد:

```
could not import sync/atomic ... export data version 4 is greater than maximum
supported version 2
```

این ناسازگاری ابزار با toolchain است، نه باگ کد. دو تشخیص آن هم **false positive** بود
(`declared and not used: r` در دو تست، در حالی که `r` در خط بعد استفاده می‌شود و `go vet`
پاک است). پس گیت lint **advisory می‌ماند** — روشن کردن گیتی که نمی‌توانم سبز بودنش را
اثبات کنم، بدتر از خاموش گذاشتنش است.

---

### ۴.۹۲ چهار کامپوننت بی‌استفاده — قرارداد کاملاً پاک — کامیت `docs(contract-zero-warnings)`

تصمیم: **وصل کردن آن‌هایی که داده‌ای واقعی توصیف می‌کنند، حذف آن‌هایی که قابلیتی
ناموجود را وعده می‌دهند.**

**وصل شد:**
- **`RefundStatus`** → schema تازهٔ `Refund` ساخته شد و پاسخ `POST /admin/orders/{}/refunds`
  به آن وصل شد. پیش از این **پاسخ بازپرداخت هیچ schema‌ای نداشت** — فقط ورودی مستند بود.
- **`ArticleStatus`** → schema تازهٔ `AdminArticleView` (نمای تحریریه با وضعیت گردش کار) و
  پاسخ `PATCH /admin/articles/{articleId}` به آن وصل شد.

**حذف شد:**
- **`AcceptLanguage`** — هیچ endpoint‌ای این header را اعلام نمی‌کرد و API تک‌زبانه است.
- **`SessionInfo`** — مسیر `/auth/sessions` هیچ‌گاه mount نشده بود؛ سندی برای قابلیتی که
  وجود ندارد.

**پنج خواندن پنل که در جابه‌جایی قبلی گم شده بودند برگشتند.** هنگام حذف ورودی‌های تکراری
`{id}`، متد `GET` این پنج مسیر هم با آن‌ها رفت: `/admin/orders/{orderNumber}/refunds`,
`/admin/products/{productId}`, `/admin/suppliers/{supplierId}/evaluations`,
`/admin/variants/{variantId}/cost-records`, `/admin/variants/{variantId}/price-records`.
گیت دوطرفه مسیرها **همان‌جا قرمز شد** و مانع از کامیت شدنش شد — یعنی گیت کار کرد.

**یک باگ در ابزار خودم:** regex من `\w` بود و `admin-catalog` (با خط تیره) را نمی‌دید،
پس هشدار tag را «صفر» گزارش می‌کرد در حالی که دو tag غایب بود. با `[\w-]` اصلاح شد.

**نتیجه:**
```
spectral:   0 errors, 0 warnings          (از ۴ هشدار اولیه)
paths:      115 · operationIds: 126
routes:     exit 0 (دوطرفه)
conformance: exit 0
```

---

### ۴.۹۳ PATCH آدرس واقعاً partial شد — کامیت `fix(address-patch)`

واگرایی ثبت‌شده در §۴.۹۰ بسته شد: مسیر `PATCH /me/addresses/{id}` بود ولی handler آدرس
**کامل** می‌خواست (`province`, `city`, `address_line`) — یعنی در عمل `PUT` بود.

**اصلاح:**
- `addressPatch` با فیلدهای **اشاره‌گر** — `nil` یعنی «دست نزن»، که معنای PATCH است
- `apply` تغییر را روی آدرس ذخیره‌شده اعمال می‌کند و **نتیجه** را اعتبارسنجی می‌کند، نه فقط
  ورودی را: پس یک partial update نمی‌تواند فیلد الزامی را خالی بگذارد
- ردیف فعلی از `ListAddresses` خوانده و با `id` پیدا می‌شود؛ نبودش → **۴۰۴**
- بدنهٔ خراب → ۴۰۰ (مثل قبل)

**تست:**
- `{"city":"کرج"}` → ۲۰۰ و **`province` و `address_line` دست‌نخورده**
- `{"city":""}` → ۴۰۰ (خالی‌کردن فیلد الزامی رد می‌شود)
- id ناشناخته → ۴۰۴
- بدنهٔ خراب → ۴۰۰

**پوشش identity: ۷۵.۴٪ → ۷۵.۳٪** (کاهش ناچیز چون کد تازه اضافه شد؛ بالای آستانه ماند).

---

### ۴.۹۴ inventory — نگاشت خطا و شاخه‌های رزرو — کامیت `test(inventory-errors)`

آستانهٔ inventory **۹۰٪** است (منطق مالی/موجودی)، نه ۷۵٪.

**`TestErrorMappingPinsStatusCodes`** — `writeInvSvcErr` ۳۳٪ بود:
- هر sentinel به کد وضعیت درست نگاشت می‌شود (۴۰۴/۴۰۹/۴۰۰) و خطای ناشناخته → **۵۰۰**
- **هر شاخه JSON برمی‌گرداند**، نه بدنهٔ خالی

**`TestHandlersSurfaceStoreFailures`** — `writeAdminErr` و `writeInvErr` **۰٪** بودند:
- pool بسته‌شده → خطای واقعی درایور → **۵۰۰ با بدنهٔ JSON**، نه panic و نه ۲۰۰ خالی
- مسیر ادمین هم بدنه دارد

**`TestGetStockBalanceHandler`** — بدون `warehouse_id` → ۴۰۰؛ موجودی ناشناخته → 4xx

**`TestPG_ReservationServiceEdges`** — `ReleaseReservation` ۷۳٪، `ConvertReservation` ۷۳٪،
`ListMovements` ۶۶٪:
- گاردها: رزرو ناشناخته → `ErrReservationNotFound`
- **release واقعاً `reserved_quantity` را صفر می‌کند** و حرکت **منفی** ثبت می‌کند
- **release دوم رد می‌شود** (`ErrReservationNotActive`)
- **convert `on_hand` را کم می‌کند** (۱۰ → ۷) و `reserved` را صفر می‌کند
- **convert دوم رد می‌شود**
- سقف `limit` (۰ و ۵۰۰۰) به بازهٔ مجاز می‌افتد

**سه اصطکاک با schema که فقط با اجرا معلوم شد:**
- `Warehouse` فیلد `WarehouseCode` دارد نه `Code`، و `Status` دارد نه `IsActive`
- `variant_id` در `stock_balances` و `inventory_reservations` **FK به `product_variants`**
  است — باید variant واقعی seed شود
- `testpg.Setup` برای pool بسته‌شده cleanup را رد می‌کند؛ در آن تست cleanup عمداً رد شد

**پوشش: inventory ۷۸.۴٪ → ۸۱.۵٪ → ۸۲.۳٪** — **آستانهٔ ۹۰٪ هنوز برآورده نشده.**
شکاف باقی‌مانده در handlerهای نوشتن است: `CreateWarehouseHandler` ۷۱٪،
`ReceiveStockHandler` ۶۸٪، `ConvertReservationHandler` ۶۷٪، `parsePositiveInt` ۳۳٪.

---

### ۴.۹۵ inventory: چهار handler نوشتن — کامیت `test(inventory-write-handlers)`

**`TestPG_WriteHandlers`:**
- **ساخت انبار:** بدنهٔ خراب و نام غایب → ۴۰۰؛ ساخت → ۲۰۱ و **وضعیت به ACTIVE پیش‌فرض شد**
  (از پایگاه داده تأیید شد)؛ کد تکراری → 4xx
- **دریافت موجودی — گارد actor (IAM §۷):** بدون actor → **۴۰۱** و **هیچ موجودی جابه‌جا
  نشد**؛ با actor → ۲۰۰ و حرکت **همان actor را ثبت کرد**؛ مقدار صفر → ۴۰۰
- **تعدیل:** بدون actor → ۴۰۱؛ `delta:-2` → ۲۰۰ و `on_hand` ۱۲ → ۱۰؛ `delta:0` → ۴۰۰
- **تبدیل رزرو:** ۲۰۰ و `on_hand` ۱۰ → ۷؛ تبدیل دوم → ۴۰۹؛ id ناشناخته → ۴۰۴
- **`parsePositiveInt`** با `""`, `abc`, `0`, `-5`, `7` — همه ۲۰۰

**باگ پیدا شده — سه خطای کلاینت که ۵۰۰ می‌گرفتند:**

```go
return fmt.Errorf("inventory: receive qty must be > 0")            // → 500
return "", fmt.Errorf("inventory: order_item_id required")         // → 500
return "", fmt.Errorf("inventory: reservation quantity must be > 0") // → 500
```

هر سه خطای **ورودی** بودند ولی sentinel نداشتند، پس `writeInvSvcErr` به شاخهٔ default
می‌افتاد. `ErrZeroQuantity` از قبل وجود داشت و نگاشت شده بود — فقط برنگردانده می‌شد.
`ErrOrderItemRequired` تازه اضافه شد (**sentinel جداگانه**، نه استفادهٔ دوباره از
`ErrReservationExpiryReq` — همان اشتباهی که در suppliers اصلاح کردم).

**`TestPG_ReserveHandlerAndAdminReads`:**
- رزرو بدون `order_item_id` → ۴۰۰ (قبلاً ۵۰۰)
- **رزرو بیش از موجودی → ۴۰۹** (ناوردای sellable)
- `expires_at` نامعتبر نادیده گرفته می‌شود، نه ۵۰۰
- فهرست رزرو ادمین: `expires_before` نامعتبر → ۴۰۰ با کد `invalid_request`
- خواندن موجودی: `on_hand=5`, `reserved=2` — واقعی

**یک واقعیت schema:** `stock_movements.created_by` **FK به `admin_users`** است — actor
باید کاربر واقعی باشد، نه رشتهٔ دلخواه.

**پوشش: inventory ۸۲.۳٪ → ۸۵.۸٪ → ۸۷.۲٪** — **آستانهٔ ۹۰٪ هنوز نه.** شکاف باقی‌مانده:
`ReserveStock` ۶۹.۶٪، `ConvertReservation` ۷۸.۹٪، `writeMovementErr` ۵۰٪.

---

### ۴.۹۶ inventory به آستانهٔ ۹۰٪ رسید — کامیت `test(inventory-threshold)`

**`TestPG_ReserveStockConcurrency`** — تنها جای ماژول که **رقابت واقعی** دارد:
- پنج goroutine هر یک ۱ واحد از موجودی ۳ می‌خواهند
- نتیجه: **۳ موفق، ۲ با `ErrInsufficientStock`** — نه بیشتر، نه کمتر
- `SELECT ... FOR UPDATE` واقعاً سریال‌سازی می‌کند و ناوردای sellable
  (`on_hand - reserved >= 0`) حفظ می‌شود

**`TestPG_ReserveStockCreatesAZeroRow`** — و یک **کامنت غلط در کد**:
کامنت می‌گفت «ردیف صفر ساخته می‌شود و بعد ناوردا شکست می‌خورد»، ولی INSERT داخل تراکنش
است و با `defer tx.Rollback` **برمی‌گردد** — ردیفی باقی نمی‌ماند. رفتار **درست‌تر** است
(رزرو ردشده نباید ردیف جا بگذارد)، پس **کامنت اصلاح شد و رفتار واقعی تثبیت شد** — نه
برعکس.

**`TestPG_ExpireReservationsAndZeroBalance`:**
- رزرو منقضی → sweep آزادش می‌کند، `reserved` صفر می‌شود و **حرکت RELEASE با ‎-2‎** ثبت
  می‌شود
- sweep دوم چیزی پیدا نمی‌کند
- واریانت بدون ردیف → **موجودی صفر**، نه ۴۰۴

**`TestPG_RecordMovementHandler`:**
- هر سه فیلد الزامی → ۴۰۰؛ مقدار صفر → ۴۰۰
- حرکت واقعی → ۲۰۱ و **`reference_type` به MANUAL پیش‌فرض شد**
- برداشت بیش از موجودی → **۴۰۹**

**`TestPG_ServiceSurfacesDriverErrors`** — ۹ نقطهٔ ورود سرویس روی pool بسته:
همه **خطای درایور برمی‌گردانند**، نه panic و نه بلعیدن. این همان شاخه‌های `return err` بعد
از هر کوئری بودند که هیچ تست خوش‌بینانه‌ای به آن‌ها نمی‌رسد.

**پوشش: inventory ۸۷.۲٪ → ۸۸.۳٪ → ۸۸.۷٪ → ۸۹.۵٪ → ۹۲.۰٪** — آستانهٔ ۹۰٪ برآورده شد.

**نکتهٔ گیت:** `audit-coverage.py --fail` **بدون** `KH_TEST_PG_DSN` قیمت‌گذاری را ۴۱.۱٪
می‌بیند و قرمز می‌شود؛ با DSN همان ۹۰.۰٪ است. این رفتار درست است (تست‌های DBStore فقط با
پایگاه داده اجرا می‌شوند) ولی باید بدانید اجرای محلی بدون DSN گیت را قرمز نشان می‌دهد.

---

### ۴.۹۷ commerce: سطح پرداخت — و یک یافتهٔ امنیتی جدی — کامیت `test(commerce-payment-http)`

**`TestPG_PaymentHTTPHandlers`** — `GetOrderByNumberHandler` ۶۷٪،
`CreatePaymentIntentHandler` ۶۲٪، `ConfirmPaymentHandler` ۶۲٪، `ListRefunds` ۶۰٪:
- شمارهٔ سفارش → ۲۰۰ و همان id؛ شمارهٔ ناشناخته → ۴۰۴
- ایجاد intent → ۲۰۱؛ سفارش ناشناخته → ۴۰۴
- **تکرار webhook → ۲۰۰ با `already_paid`** (قرارداد idempotent)
- سفارش واقعاً به PAID رفت
- فهرست بازپرداخت سفارش بدون بازپرداخت → **لیست خالی**، نه ۴۰۴

## ⚠️ یافتهٔ امنیتی — تصمیم مالک لازم است

هنگام نوشتن تست، انتظار داشتم «تأیید پرداخت با intent پرداختنشده» رد شود. **نشد:**

```
POST /api/v1/orders/{id}/confirm-payment   → 200 {"status":"paid"}
```

**علت:** مسیر روی `api.public` mount شده، **بدون امضا و بدون احراز هویت**، و
`ConfirmPaymentForOrder` وضعیت intent را `REQUIRES_PAYMENT` یا `PROCESSING` می‌پذیرد:

```go
if pi.Status != IntentRequiresPayment && pi.Status != IntentProcessing {
    return ErrIntentNotSucceeded
}
```

یعنی **هر کسی می‌تواند سفارش خودش را PAID کند بدون اینکه پولی رسیده باشد.** مسیر
HMAC-دار و درست `POST /api/v1/payments/webhook` است که امضا را با
`PaymentWebhookSecret` بررسی می‌کند — این مسیر آن را دور می‌زند.

**تصمیم مالک لازم است** (سیاست امنیتی): امضا اجباری شود، توکن کارمند لازم شود، یا مسیر
حذف شود؟ تا آن زمان **رفتار فعلی در تست تثبیت شد** تا بی‌صدا تغییر نکند.

**پوشش: commerce ۸۳.۶٪ → ۸۴.۱٪** — آستانهٔ ۹۰٪ هنوز نه.

---

### ۴.۹۸ بستن یافتهٔ امنیتی — امضای اجباری روی تأیید پرداخت — کامیت `fix(confirm-payment-signature)`

تصمیم مالک: **گزینهٔ ۱** — امضای اجباری.

`ConfirmPaymentHandler` حالا **بدنهٔ خام را می‌خواند و امضای HMAC را با همان کلید مسیر
`/payments/webhook` بررسی می‌کند**، پیش از هر کاری:

```go
if !VerifyWebhookSignature(secret, body, r.Header.Get("X-Provider-Signature")) {
    writeJSON(w, http.StatusUnauthorized, {"code": "invalid_signature"})
    return
}
```

امضا روی **بدنهٔ خام** است (نه روی id) تا همان مکانیزم webhook بازاستفاده شود و کلاینت
نتواند id را عوض کند.

**شاهد — سه حالت آزموده شد:**
```
فراخوانی بدون امضا        → ۴۰۱ invalid_signature
امضای غلط (deadbeef)      → ۴۰۱
وضعیت سفارش پس از رد      → PENDING_PAYMENT (دست‌نخورده)
امضای درست                → ۲۰۰ {"status":"paid"} و سفارش PAID
تکرار با امضای درست       → ۲۰۰ {"status":"already_paid"}
```

**قرارداد به‌روز شد:** پارامتر `X-Provider-Signature` (header، required) با توضیح
HMAC-SHA256 بدنهٔ خام، و توصیف فارسی مسیر از «مسیر پنل/ویترین» به «اعلان درگاه» تغییر کرد.

**دو تست قدیمی که رفتار ناامن را تثبیت می‌کردند** (`TestHTTP_ConfirmPayment_ReturnsPaid`,
`..._IdempotentOnSecondCall`) حالا با `signedRequest` امضا می‌فرستند — تست سبز بود ولی
قرارداد امنیتی غلط را قفل کرده بود.

**پوشش: commerce ۸۴.۱٪** (بدون تغییر معنادار؛ آستانهٔ ۹۰٪ هنوز نه).

---

### ۴.۹۹ commerce: مسیرهای checkout و کارت — کامیت `test(commerce-paths)`

**`TestPG_CheckoutPaths`** — `Checkout` ۷۰.۷٪:
- **مهمان بدون شماره تلفن → `ErrGuestPhoneRequired`** (پیش از هر قیمت‌گذاری)
- آدرس غایب → `ErrShippingAddressReq`
- **مهمان که آیتم مستقیم می‌فرستد:** آیتم‌ها در سبد تاشده و **یک بار** قیمت می‌خورند؛
  سفارش ساخته می‌شود و `GrandTotal` درست است
- **مسیر `CartID` صریح:** سبد مشتری، سپس checkout با id سبد
- دو checkout دو **شمارهٔ سفارش متفاوت** می‌دهد
- handler برای مهمان بدون تلفن → 4xx نه ۵۰۰

**`TestPG_ServiceSurfacesDriverErrors`** — ۱۵ نقطهٔ ورود سرویس روی pool بسته:
همه خطای درایور برمی‌گردانند. این همان شاخه‌های `return err` بعد از هر کوئری بودند.

**`TestPG_CartServicePaths`** — سبد مهمان، خواندن با توکن، **توکن ناشناخته سبد نمی‌سازد**،
به‌روزرسانی تعداد، حذف قلم، و **ادغام مهمان که تعدادها را جمع می‌کند** (۱ + ۳ = ۴) و
**ادغام دوم تکرار نمی‌شود** (سبد مهمان CONVERTED شده).

**پوشش: commerce ۸۴.۱٪ → ۸۴.۵٪ → ۸۶.۰٪.**

## تصحیح یک ادعای خودم

در پیام قبلی `commerce` را «آخرین آستانهٔ سخت» خواندم. **غلط بود:**

```
ENFORCED = {'pricing'}          # تنها آستانهٔ سخت
commerce  86.0% / 90%  below [advisory]
```

`commerce` **advisory** است، نه enforced. گیت سخت فقط `pricing` است و آن ۹۰.۰٪ و سبز است.
پس `audit-coverage.py --fail` **exit 0** می‌دهد.

---

### ۴.۱۰۰ commerce: مرجوعی، بازپرداخت و خواندن سفارش — کامیت `test(commerce-refund-paths)`

**`TestPG_CreateRefundGuards`** — `CreateRefund` ۷۵٪:
- **ترتیب گاردها تثبیت شد:** store سیم‌نشده → `ErrRefundStoreNotWired`؛ مقدار ≤۰ →
  `ErrRefundAmountInvalid`؛ دلیل خالی → `ErrRefundReasonReq`؛ سفارش ناشناخته →
  `ErrOrderNotFound`
- **بازپرداخت بیش از پرداخت → `ErrRefundExceedsPaid`**
- **بازپرداخت جزئی سفارش را نمی‌بندد**؛ بازپرداخت باقی‌مانده → `refunded_total=100000` و
  وضعیت **CLOSED**
- **بازپرداخت سوم با گارد وضعیت رد می‌شود** (`ErrOrderNotRefundable`) — نه با گارد مقدار.
  این ترتیب را با اجرا فهمیدم؛ فرض اولم غلط بود.
- فهرست بازپرداخت هر دو را برمی‌گرداند

**`TestPG_ReturnRequestPaths`** — `returns.go`:
- دلیل خالی → `ErrReturnReasonRequired`؛ مقدار صفر → خطا
- **مرجوعی برای سفارش دیگری رد می‌شود**؛ سفارش ناشناخته → `ErrOrderNotFound`

**`TestPG_OrderReadPaths`** — `GetOrderHandler` ۷۲٪:
- **خواندن ناشناس سفارش مهمان → ۴۰۳، نه ۴۰۴** — سفارش وجود دارد ولی خواننده مالکش نیست
  (حریم خصوصی، نه enumeration)
- مسیر با شمارهٔ تجاری هم همان نتیجه را می‌دهد
- شماره/uuid ناشناخته → ۴۰۴؛ سفارش با نشست مشتری دیگر → ۴۰۳
- `FindIntentByProviderReference` با مرجع ناشناخته → idهای خالی، نه خطا

**`TestPG_StoreSurfacesDriverErrors`** — ۲۲ متد `DBStore` روی pool بسته: همه خطا
برمی‌گردانند. این شاخه‌های `return err` بودند که هیچ تست خوش‌بینانه‌ای نمی‌دید.

**پوشش: commerce ۸۶.۰٪ → ۸۶.۲٪ → ۸۷.۴٪ → ۸۷.۶٪** — **آستانهٔ ۹۰٪ (advisory) هنوز نه.**

---

### ۴.۱۰۱ commerce: outbox، فهرست ادمین و اعتبارسنجی — کامیت `test(commerce-outbox-validation)`

**`TestPG_ConfirmEmitsOrderPaid`:**
- تأیید پرداخت **رخداد `OrderPaid` را در outbox می‌گذارد**
- **تکرار تأیید رخداد دومی نمی‌فرستد** (`ErrAlreadyPaid`)
- **خرابی صف تأیید را نمی‌شکند** — پول جابه‌جا شده و outbox بهترین‌تلاش است (P4)

**`TestPG_ListOrdersHandlerPaging`** — `ListOrdersHandler` ۷۵٪:
- فیلتر `status` و `q`؛ صفحه‌بندی بی‌معنا → پیش‌فرض؛ `page_size=5000` → ۲۰ (خارج از بازه
  به پیش‌فرض برمی‌گردد، نه سقف)
- **شاخهٔ ۵۰۰** با pool بسته، با بدنهٔ JSON

**`TestPG_MergeCartsStoreEdge`:**
- **ادغام سبد با خودش no-op است**، نه خطا؛ سبد سالم می‌ماند
- `CartView` با id ناشناخته → `ErrCartNotFound`
- حذف قلم ناشناخته → خطا

**باگ پیدا شده — ۵۰۰ به‌جای ۴۰۱:**

```
POST /api/v1/cart/merge بدون نشست مشتری → 500
```

`MergeGuestCart` یک `errors.New(...)` خام برمی‌گرداند، پس `statusForError` به default
می‌افتاد. `ErrCustomerSessionRequired` اضافه شد → **۴۰۱** با کد پایدار. همان کلاس باگ‌های
خام inventory.

**`TestPG_OrderInputValidationAndMyOrders`:**
- هر شاخهٔ `validateOrderInput`: تماس غایب، آیتم خالی، `supply_type` نامعتبر، snapshot غایب
- `MyOrdersHandler` → ۲۰۰ با متادیتای صفحه‌بندی؛ صفحه‌بندی بی‌معنا → پیش‌فرض

**شاخهٔ مرده:** `ErrOrderNumberRequired` از `CreateOrder` **دست‌نیافتنی** است — شمارهٔ خالی
خودکار تولید می‌شود. تثبیت شد تا تفاوت دیده شود، نه اینکه رفتار عوض شود.

**پوشش: commerce ۸۷.۶٪ → ۸۸.۱٪ → ۸۸.۵٪ → ۸۸.۸٪** — آستانهٔ ۹۰٪ (advisory) هنوز نه.

---

### ۴.۱۰۲ commerce به ۹۰٪ رسید — کامیت `test(commerce-threshold)`

**`TestPG_HandlerPathFallbacks`** — هر handler یک شاخهٔ جایگزین دارد برای روتری که
`PathValue` ست نمی‌کند و id را از مسیر می‌خواند. این شاخه در production **کد مرده** است و
در CI تست‌نشده بود:
- `GetOrderHandler`, `CreatePaymentIntentHandler`, `CancelOrderHandler`,
  `ConfirmPaymentHandler` — همه با مسیر واقعی و **بدون** `SetPathValue`
- ۴۰۴ یعنی fallback کار نکرده؛ هیچ‌کدام ۴۰۴ نداد

**`TestPG_IdempotencyHelpers`** — `jsonBody` (۰٪): مقدار قابل marshal، و مقدار
غیرقابل marshal → بایت خالی، نه panic.

**پوشش: commerce ۸۸.۸٪ → ۸۹.۹٪ → ۹۰.۱٪** — آستانهٔ ۹۰٪ برآورده شد.

```
audit-coverage: all enforced thresholds met
below threshold but advisory: 0
```

**هیچ ماژولی زیر آستانه نمانده — نه enforced، نه advisory.**

---

### ۴.۱۰۳ دود-تست باینری واقعی — دو باگ که تست‌ها نمی‌دیدند — کامیت `fix(storefront-slug-lookup)`

تست‌ها سبز بودند، ولی **محصول اجرا نشده بود**. باینری واقعی روی `:8099` با پایگاه دادهٔ
اصلی بالا آمد و مسیر مشتری پیاده شد:

```
GET /api/v1/catalog/products?page=1&page_size=3     200
GET /api/v1/catalog/products/draft-leak-check       500   ← باگ
POST /api/v1/cart/items  {"variant_id":""}          500   ← باگ
```

**باگ ۱ — صفحهٔ محصول ویترین ۵۰۰ می‌داد.** ویترین محصول را با **slug** لینک می‌دهد
(`/products/draft-leak-check`) ولی `GetProduct` مستقیم `WHERE id = $1` روی ستون **uuid**
می‌زد → خطای درایور. با uuid کار می‌کرد، با slug نه — یعنی **هر صفحهٔ محصول در production
می‌شکست** و هیچ تستی نمی‌دید چون تست‌ها uuid می‌فرستادند.

سه جا اصلاح شد:
- `GetProduct` اگر ورودی uuid نبود، `WHERE slug = $1`
- `GetProductDetail` برای کوئری واریانت‌ها از `p.ID` استفاده می‌کند، نه کلید ورودی
- بارگذاری دسته‌بندی‌ها هم `p.ID`

**باگ ۲ — `variant_id` خالی ۵۰۰.** `errors.New` خام بدون sentinel → `ErrVariantRequired`
→ **۴۰۰**.

**شاهد پس از اصلاح (همان باینری، همان پایگاه داده):**
```
catalog/products/draft-leak-check      200
add item (variant خالی)                400
add item (variant واقعی)               200
read cart                              200  (قلم در سبد)
```

**درس:** ۳۹۹ تست سبز بود و صفحهٔ اصلی محصول شکسته. تست‌ها uuid می‌فرستادند چون سازندهٔ
تست uuid داشت؛ ویترین slug می‌فرستد. **تست فقط آن چیزی را می‌بیند که به آن می‌دهی.**

---

### ۴.۱۰۴ مسیر کامل مشتری روی باینری واقعی — و کد خطای بی‌مصرف — کامیت `fix(error-codes)`

**سفر کامل مشتری، واقعاً اجرا شد:**

```
POST /cart/items                      200
GET  /cart                            200
POST /checkout  (بدون Idempotency-Key) 400  cart_token_required ← درست
POST /checkout  (با Idempotency-Key)   201  KHA-ORD-2026-000009
     subtotal 500000 = ۲ × 250000 · status PENDING_PAYMENT
POST /checkout  (همان کلید، تکرار)     201  همان سفارش (idempotent)
POST /orders/{id}/confirm-payment (بدون امضا)  401
POST /orders/{id}/confirm-payment (با امضا)    200 {"status":"paid"}
POST /orders/{id}/confirm-payment (تکرار)      200 {"status":"already_paid"}
```

و از پایگاه داده:
```
KHA-ORD-2026-000009 | PAID | paid_total=500000 | payment_attempts=1
```
**یک attempt موفق ثبت شد** — پیش‌شرط بازپرداخت (DB §10.5) برقرار است.

**باگ پیدا شده — `code` بی‌مصرف بود:**

```json
{"error":"commerce: order is already paid","code":"COMMERCE"}
```

`errorCode` کد را با **بریدن پیام در اولین `:`** می‌ساخت. هر پیام با `commerce:` شروع
می‌شود، پس **هر خطا `"code":"COMMERCE"`** برمی‌گرداند — کلاینت نمی‌توانست روی آن شاخه
بزند. با نگاشت صریح ۳۰ sentinel جایگزین شد؛ خطای ناشناخته → `request_failed`.

**شاهد پس از اصلاح:**
```
{"error":"commerce: order is already paid","code":"already_paid"}
{"error":"commerce: variant_id is required","code":"variant_required"}
{"code":"cart_token_required",...}
```

`TestErrorCodeIsMachineReadable` این نگاشت را تثبیت می‌کند، شامل sentinel‌های **wrap‌شده**
(`fmt.Errorf("%w: status=CLOSED", …)`) که کدشان نباید با پسوند جزئیات عوض شود.

**رسانه:** `POST /admin/media` بدون احراز هویت → **۴۰۱** (درست).

---

### ۴.۱۰۵ پنل ادمین روی باینری واقعی — و ۴۰۵/۴۰۴ متنی — کامیت `fix(json-errors)`

**حساب کارمند برای دود-تست ساخته شد** (بدون نیاز به گذرواژهٔ موجود؛ هش bcrypt تازه با
گذرواژهٔ معلوم، نقش `SMOKE_ADMIN` با ۲۴ مجوز). **اعتبارنامهٔ هیچ حساب واقعی حدس زده نشد.**

```
POST /auth/admin/login                     200  access_token (۶۴ کاراکتر)
GET  /admin/orders  (با مجوز)              200
GET  /admin/stock                          200
GET  /admin/users                          200
GET  /admin/audit-logs                     200
GET  /admin/products                       200
GET  /admin/orders?q=000009                200
GET  /admin/orders  (بدون توکن)            401
GET  /admin/orders  (توکن نامعتبر)         401
GET  /admin/orders  (نقش بدون مجوز)        403  ← RBAC واقعاً کار می‌کند
```

**رسانه، رفت‌وبرگشت کامل:**
```
POST /admin/media           201  {"id":…,"media_type":"IMAGE","storage_key":"IMAGE/…"}
GET  /media/{id}/content    200  ۱۱ بایت — cmp با فایل اصلی: یکسان
GET  /media/{id}            200
POST /admin/media (بدون احراز) 401
```

**باگ پیدا شده — ۴۰۵ و ۴۰۴ متن ساده بودند:**

```
HTTP/1.1 405 Method Not Allowed
Content-Type: text/plain; charset=utf-8
Method Not Allowed
```

کلاینتی که همیشه JSON می‌خواند، روی **دقیقاً همان پاسخ‌هایی** که باید رویشان شاخه بزند
(متد اشتباه، مسیر ناشناخته) خطای parse می‌گرفت. `jsonErrors` اضافه شد و mux را می‌پوشاند:

```
HTTP/1.1 405  Content-Type: application/json; charset=utf-8
{"code":"method_not_allowed","error":"method not allowed"}
{"code":"not_found","error":"not found"}
```

`TestJSONErrors` هر دو حالت و عبور بی‌تغییر پاسخ عادی را تثبیت می‌کند.

## شکاف قابلیت (نه باگ) — تصمیم مالک

**`GET /admin/warehouses` وجود ندارد** — فقط `POST`. پنل می‌تواند انبار بسازد ولی هرگز
فهرستش را نمی‌بیند، پس `{wid}` لازم برای عملیات موجودی را از کجا بیاورد؟ کد و قرارداد
**هر دو** این مسیر را ندارند، پس واگرایی نیست؛ **قابلیت غایب** است. افزودنش تغییر قرارداد
است و تصمیم مالک می‌خواهد.

---

### ۴.۱۰۶ `GET /admin/warehouses` — شکاف بسته شد — کامیت `feat(warehouse-list)`

تصمیم مالک: افزودن مسیر فهرست.

**`AdminStore.ListWarehouses`** + **`ListWarehousesHandler`** + مسیر
`GET /api/v1/admin/warehouses` با مجوز `inventory.manage` + ورودی قرارداد.

- فیلتر `status` و جست‌وجوی `q` روی **نام و کد** انبار
- متادیتای صفحه‌بندی (`page`, `page_size`, `total`, `total_pages`)
- `created_at` به RFC3339

**شاهد روی باینری واقعی:**
```
GET /admin/warehouses  →  200
{"items":[{"id":"00de4316-…","name":"انبار تهران","warehouse_code":"WH-TEHRAN",
           "status":"ACTIVE","created_at":"2026-11-26T10:55:47Z"}],
 "page":{"page":1,"page_size":20,"total":1,"total_pages":1}}
```

**سفر کامل موجودی، حالا ممکن شد:**
```
GET  /admin/warehouses/{wid}/variants/{vid}/movements   200  حرکت RECEIPT با quantity=5
POST /admin/warehouses/{wid}/variants/{vid}/receive     200  {"ok":true}
POST /admin/warehouses/{wid}/variants/{vid}/adjust      200  delta=-2
GET  /warehouses/{wid}/variants/{vid}/stock             200  on_hand=3
GET  /admin/audit-logs?action=inventory.adjust          →  ۲ ردیف
```

پایگاه داده تأیید کرد: `on_hand=3, reserved=0` — تعدیل واقعاً اعمال شد.
**و تریل حسابرسی هر دو عمل را ثبت کرد** (IAM §۷ برای موجودی).

**یک اشتباه من:** مسیر موجودی را `/admin/warehouses/…/stock` زدم و ۴۰۴ گرفتم. مسیر واقعی
`/warehouses/…/stock` (عمومی) است — همان که ویترین می‌خواند. **باگ نبود.**

---

### ۴.۱۰۷ بازپرداخت روی باینری واقعی — ۵۰۰ روی هر بازپرداخت — کامیت `fix(refund-route-key)`

```
POST /admin/orders/KHA-ORD-2026-000009/refunds  →  500
{"error":"internal error","code":"internal_error"}
```

لاگ سرور: `invalid input syntax for type uuid: ""`

**علت:** مسیر `{orderNumber}` اعلام می‌کند، ولی هر دو handler
(`CreateRefundHandler`, `ListRefundsHandler`) `r.PathValue("id")` می‌خواندند — **خالی**.
پس `order_id` رشتهٔ تهی به ستون uuid می‌رفت.

**یعنی هر بازپرداخت در production ۵۰۰ می‌داد** — عملیات مالی اصلی، بدون استثنا.

**اصلاح:** `resolveOrderKey` — کلید مسیر را می‌خواند (`orderNumber`، با fallback به `id`)،
اگر uuid بود همان را، وگرنه شمارهٔ تجاری را به id تبدیل می‌کند.

**ترتیب مهم:** بدنه **قبل** از جست‌وجوی سفارش decode می‌شود — درخواست خراب باید ۴۰۰ بگیرد
بدون رفت‌وبرگشت پایگاه داده. تست موجود (`TestHandlerGuards`) همین ترتیب را تثبیت کرده بود
و مرا از جابه‌جا کردنش بازداشت.

**شاهد پس از اصلاح:**
```
POST /admin/orders/KHA-ORD-2026-000009/refunds   201  {"id":"44c475e0-…"}
GET  /admin/orders/KHA-ORD-2026-000009/refunds   200  status COMPLETED، ارجاع به attempt
POST … refunds (بیش از پرداخت)                   409
POST … refunds (شمارهٔ ناشناخته)                 404
```
پایگاه داده: `refunded_total=100000`، یک ردیف در `refunds`.

**تریل حسابرسی (IAM §۷) واقعاً نوشت:**
```
action: order.refund.issued | actor_type: ADMIN | actor_id: 3fe33da9… | entity: order
new_values: {"amount":100000,"status":"PAID","refund_id":"44c475e0-…","refunded_total":100000}
```

---

### ۴.۱۰۸ ۴۰۱/۴۰۳ متن ساده — کامیت `fix(json-auth-errors)`

```
GET /api/v1/me  →  401  Content-Type: text/plain
unauthorized
```

همان کلاس ۴۰۵/۴۰۴، ولی این یکی را mux نمی‌نویسد بلکه گاردهای `authorization` — پس
`jsonErrors` نمی‌گرفتش.

دو جا اصلاح شد:
- `writeAuthErr` (که ۴۰۱/۴۰۳ را متنی می‌نوشت) → JSON با کد
- `Require` که از `http.Error` استفاده می‌کرد → همان `writeAuthErr`

**شاهد:**
```
HTTP/1.1 401 Unauthorized
Content-Type: application/json; charset=utf-8
{"code":"unauthorized","error":"unauthorized"}
```

`TestWriteAuthErrIsJSON` و `TestRequireAnswersJSON` هر دو حالت (بدون هویت → ۴۰۱، مجوز
ردشده → ۴۰۳) و وجود هدر `WWW-Authenticate` را تثبیت می‌کنند.

**پوشش authorization: ۷۶.۱٪ → ۷۶.۹٪**

---

### ۴.۱۰۹ مسیر مشتری روی باینری واقعی — سه باگ — کامیت `fix(customer-surface)`

**باگ ۱ — ورود مشتری روی واحد API وجود نداشت.** `/auth/otp/*` فقط در `runIdentity` ثبت
شده بود؛ در استقرار تک‌واحدی **۴۰۴**. یعنی **هیچ مشتری نمی‌توانست وارد شود** — `/me`،
آدرس‌ها، سفارش‌ها و مرجوعی همه دور از دسترس. همان کلاس باگ ورود کارمند (§۴.۶۸).
→ چهار مسیر OTP در `runAPI` هم mount شد. **شاهد: `POST /auth/otp/request` → ۲۰۲،
`verify` → ۲۰۰ با توکن ۶۴ کاراکتری.**

**باگ ۲ — مشتری واردشده مهمان حساب می‌شد.** `/cart` و `/checkout` روی `api.public` بودند
که هیچ هویتی را نمی‌خواند:

```
GET  /cart   با توکن مشتری  →  400  cart_token_required
POST /checkout با توکن مشتری →  400  guest_phone_required
```

`authorization.OptionalCustomer` و `apiMux.publicCustomer` اضافه شد: اگر توکن مشتری معتبر
بود، هویت را می‌چسباند، وگرنه مهمان می‌ماند. **شاهد: `GET /cart` → ۲۰۰،
`POST /cart/items` → ۲۰۰، `POST /checkout` → ۲۰۱ (KHA-ORD-2026-000010)، `/me/orders` → ۲۰۰
با total=1.**

**باگ ۳ — سفارش DELIVERED قابل مرجوع نبود.** `returnableStatuses` شامل PAID،
IN_FULFILLMENT، SHIPPED و COMPLETED بود ولی **DELIVERED نه** — یعنی کالایی که به دست
مشتری رسیده، قابل مرجوع نبود:

```
POST /orders/{id}/return-requests  →  409  return_not_allowed
```
→ `OrderDelivered` اضافه شد. **شاهد: ۲۰۱ و ردیف `REQUESTED` در `return_requests`.**

**یافتهٔ عملیاتی:** پایگاه دادهٔ اصلی **یک migration عقب بود** — `return_requests`
(000016) وجود نداشت ولی `idempotency_keys` (000015) وجود داشت. migration به‌صورت
forward-only اعمال شد. **اگر این را نمی‌دیدم، هر مرجوعی با `relation does not exist` ۵۰۰
می‌داد.**

---

### ۴.۱۱۰ چرخهٔ مرجوعی — قابلیت نیمه‌کاره — کامیت `feat(return-transitions)`

**یافته:** مشتری می‌توانست درخواست مرجوعی ثبت کند و **هیچ راهی برای پردازشش وجود نداشت**.
پنج وضعیت تعریف شده بود (`REQUESTED`, `APPROVED`, `REJECTED`, `RECEIVED`, `REFUNDED`) ولی
هیچ متد انتقال و هیچ مسیر ادمینی نبود. درخواست تا ابد در `REQUESTED` می‌ماند.

**افزوده شد:**
- `returnTransitions` — تنها نقشهٔ مجاز: `REQUESTED → APPROVED → RECEIVED → REFUNDED`،
  `REQUESTED → REJECTED`. هر چیز دیگر ۴۰۹.
- `TransitionReturn` — `UPDATE … WHERE id=$1 AND status=$2`؛ وضعیت مبدأ در WHERE است، پس
  **دو تصمیم همزمان نمی‌توانند هر دو موفق شوند** — بازنده صفر ردیف می‌گیرد و ۴۰۹.
- `DecideReturn` / `ReceiveReturn` + سه مسیر ادمین با مجوز `order.manage`.
- رد بدون یادداشت ۴۰۰ است: مشتری‌ای که درخواستش رد شده، مستحق دلیل است.

**شاهد روی باینری:**
```
POST …/receive  قبل از approve   409  invalid_return_transition (current REQUESTED)
POST …/reject   بدون یادداشت     400  decision_note_required
POST …/approve                   200  status APPROVED، note «تأیید»
POST …/approve  دوباره           409
POST …/receive                   200  status RECEIVED
```
پایگاه داده: `RECEIVED|رسید`

قرارداد: ۱۳۰ مسیر mount، ۱۳۰ اعلان‌شده، **۰ MOUNTED BUT NOT IN THE CONTRACT**.
پوشش commerce: ۸۸.۸٪ → **۸۸.۷٪** (با ۳۰۹ خط جدید پوشیده‌شده).

**بازپرداخت گام جداگانه ماند** — `RECEIVED → REFUNDED` عمداً به `CreateRefund` وصل نشد،
چون جابه‌جایی پول باید همان مسیر حسابرسی‌شدهٔ بازپرداخت را طی کند، نه یک مسیر موازی.

---

### ۴.۱۱۱ تریل حسابرسی برای تغییر موفق — شکاف ۶۷ از ۷۱ — کامیت `feat(central-audit)`

**یافته:** `RequireAudited` فقط **ردها** را حسابرسی می‌کرد. هیچ تغییر موفقی از خود گارد
تریل نداشت — یعنی می‌شد پرسید «چه کسی رد شد» ولی نه «چه کسی این قیمت را عوض کرد».

**اصلاح مرکزی:** گارد حالا هر تغییر موفق (`POST/PUT/PATCH/DELETE` با ۲xx) را ثبت می‌کند:

```
action: admin.post | actor_type: ADMIN | permission: order.manage
status: 200 | path: /api/v1/admin/return-requests/f9e157af-…/approve
```

**سه ویژگی که این را از یک patch ساده جدا می‌کند:**
1. **فراموش‌نشدنی است** — مسیر ادمین جدید بدون تریل منتشر نمی‌شود، چون گارد آن را می‌نویسد نه
   handler.
2. **فقط موفق** — `statusRecorder` وضعیت واقعی را نگه می‌دارد؛ درخواست ۴۰۹/۴۰۴ ردیف
   «انجام شد» نمی‌سازد. شاهد: دو درخواست ناموفق (۴۰۴ و ۴۰۹) صفر ردیف نوشتند.
3. **مقادیر قبل/بعد همچنان کار handler است** — این لایه ثبت می‌کند که «اقدام رخ داد»، نه
   اینکه چه چیزی عوض شد. بازپرداخت و موجودی تریل غنی خودشان را دارند.

**یک تست قدیمی باگ را تثبیت کرده بود:** `TestRequireAuditedAllowsWhenPermitted` انتظار
داشت درخواست مجاز **صفر** ردیف بنویسد. اصلاح شد به «دقیقاً یک ردیف، و نه از نوع رد».

---

### ۴.۱۱۲ صف مرجوعی — API و پنل — کامیت `feat(returns-queue)`

**یافته:** سه مسیر انتقال وجود داشت ولی **هیچ مسیری برای دیدن صف نبود**. کارمند فقط
می‌توانست درخواستی را پردازش کند که id آن را از قبل می‌دانست. یعنی چرخهٔ §۴.۱۱۰ در عمل
قابل استفاده نبود.

**افزوده شد:**
- `GET /api/v1/admin/return-requests?status=&page=&page_size=` با مجوز `order.view`
- وضعیت ناشناخته ۴۰۰ است، نه فهرست خالی — «NOPE» نباید مثل «همه» رفتار کند
- `ReturnsScreen` در پنل: فیلتر وضعیت، تأیید/رد/ثبت دریافت، یادداشت تصمیم
- سه تابع کلاینت در `web/shared/api.ts`

**شاهد روی باینری:**
```
GET /admin/return-requests               200  total=2 (APPROVED، RECEIVED)
GET /admin/return-requests?status=APPROVED 200  total=1
GET /admin/return-requests?status=NOPE   400
GET /admin/return-requests (بدون توکن)   401
```

پنل: `tsc --noEmit` پاک، بیلد admin و shop سبز.
قرارداد: **۱۳۱ mount، ۱۳۱ اعلان‌شده، ۰ بدون قرارداد**.

---

### ۴.۱۱۳ تصمیم مالک — گزینهٔ «ج» برای هر دو — کامیت `feat(login-throttle-return-refund)`

**۱) محدودیت نرخ ورود کارمند + تریل**

`AdminLoginHandler` هیچ محدودیتی نداشت: OTP سقف ۵/ساعت داشت ولی گذرواژهٔ کارمند بینهایت
قابل حدس بود. `AdminLoginGuard` اضافه شد با **دو سقف** — ۱۰ در ۱۵ دقیقه بر اساس IP و ۵ بر
اساس ایمیل. دو ابعاد عمدی است: سقف IP تنها به بات‌نت اجازهٔ پخش می‌دهد، سقف ایمیل تنها به
مهاجم اجازهٔ قفل کردن همکار را می‌دهد.

محدودیت **قبل** از bcrypt بررسی می‌شود — وگرنه خود محدودکننده به ابزار DoS تبدیل می‌شود.

**شاهد:**
```
تلاش ۱..۵  → 401
تلاش ۶     → 429
audit_logs: auth.admin_login_failed | throttled          | smoke-admin@khabavar.ir
            auth.admin_login_failed | invalid_credentials | smoke-admin@khabavar.ir
```

**۲) اتصال `RECEIVED → REFUNDED` با مبلغ ورودی کارمند**

`RefundReturn` بازپرداخت را از همان مسیر حسابرسی‌شدهٔ `CreateRefund` می‌فرستد و سپس درخواست
را می‌بندد. **ترتیب عمدی:** پول اول، چون درخواستِ بسته‌شده بدون پول دروغی است که مشتری
می‌بیند؛ اگر بستن شکست بخورد، خطا شناسهٔ بازپرداخت را می‌آورد تا اپراتور تطبیق دهد نه اینکه
دوباره بپردازد.

**شاهد کامل روی باینری:**
```
POST …/refund {"amount":0}       400  invalid_amount
POST …/refund {"amount":999999}  409  refund_exceeds_paid
POST …/refund {"amount":40000}   200  status REFUNDED
POST …/refund دوباره             409
پایگاه داده: REFUNDED | refunded_total=40000
```

**باگ ششم از کلاس نگاشت:** `ErrNoSucceededPayment` (سفارش هرگز پرداخت نشده) به **۵۰۰** نگاشت
می‌شد، نه ۴۰۹. خطای کسب‌وکار به‌صورت «internal error» به کلاینت می‌رسید و علت واقعی را
پنهان می‌کرد. به گروه ۴۰۹ اضافه شد.

قرارداد: **۱۳۲ mount، ۱۳۲ اعلان‌شده، ۰ بدون قرارداد**.

---

### ۴.۱۱۴ بازرسی سیستماتیک — سه یافتهٔ تازه — کامیت `fix(api-consistency)`

**روش:** به‌جای آزمون دستی، یک کاوشگر روی **همهٔ ۱۳۲ مسیر قرارداد** با بدنه‌های دشمنانه
(`{}`، `not-json`، `[]`، `null`، مبلغ منفی، تعداد صفر، رشتهٔ ۴۰۰۰ کاراکتری در ۱۷ فیلد،
`page_size=999999`) و شناسه‌های **واقعی** از پایگاه داده.

**نتیجهٔ کاوش (سالم):** صفر خطای ۵xx. صفر مسیر ادمین بدون توکن که باز شود. صفر مشکل با
`page=0`/`page_size=999999`/`page=abc`. صفر مشکل با نبود `Content-Type`.

**یافتهٔ ۱ — کلید idempotency بلند → ۵۰۰.** ستون `idempotency_keys.key` برابر
`varchar(100)` است؛ کلید ۲۰۰۰ کاراکتری به پایگاه داده می‌رسید و «internal error» برمی‌گشت
برای چیزی که plainly بدنهٔ خراب است. گارد `maxIdempotencyKeyLen` پیش از لمس پایگاه داده.

**یافتهٔ ۲ — دو پاکت خطا.** `identity.writeErr` فقط `{"error":…}` می‌نوشت، بدون `code` — در
حالی که بقیهٔ API `{"code":…,"error":…}` می‌دهد. کلاینتی که روی `code` شاخه می‌زند (همان
کاری که بقیهٔ API به آن یاد می‌دهد) از مسیرهای هویت هیچ نمی‌گرفت. `machineCode` کد پایدار
می‌سازد: `"email and password required"` → `email_and_password_required`.
**۲۹ فراخوانی** یک‌جا درست شد.

**یافتهٔ ۳ — OIDC روی واحد API وجود نداشت.** `/oauth/authorize` و `/oauth/token` فقط در
`runIdentity` بودند؛ در استقرار تک‌واحدی **۴۰۴**. صفحهٔ ورود پنل با `beginOIDCLogin` شروع
می‌شود، پس **ورود پنل کار نمی‌کرد**. سومین نمونهٔ «فقط در یک واحد mount شده» پس از ورود
کارمند و OTP.

**شاهد پس از اصلاح:**
```
GET  /oauth/authorize (بدون پارامتر)  400  {"error":"invalid_request"}
GET  /oauth/authorize (کامل)          400  {"error":"invalid_client"}
POST /oauth/token                     400  {"error":"invalid_request"}
```

قالب `{"error":…}` در OIDC **عمداً** دست‌نخورده ماند: RFC 6749 همین را الزام می‌کند.

---

## ۵. کارهای باقی‌مانده (به ترتیب اولویت)

> این بخش در ۲۰۲۶-۱۱-۲۷ **بازنویسی شد**. نسخهٔ قبلی سه ادعای کهنه داشت که با خروجی
> ابزار رد شدند: «operationId وجود ندارد» (۵۸ مورد از قبل بود)، «magazine و admin هنوز
> scaffold‌اند» (هر دو ساخته و سبز)، و «audit فقط برای بازپرداخت» (هفت از هفت دسته اکنون
> پوشش دارد). سندی که وضعیت غلط گزارش کند همان چیزی است که R-19 §۹ منع می‌کند.

### بسته‌شده (با شاهد)

1. **سطح HTTP قرارداد** — `scripts/audit-routes.py`: بخش «DECLARED BUT NOT MOUNTED»
   از خروجی حذف شد، یعنی **۵۸ از ۵۸** مسیر قرارداد mount است.
2. **فرانت‌اند** — چهار اپ (`main`, `shop`, `admin`, `magazine`) بیلد سبز؛ Storybook و
   دروازهٔ axe (`scripts/a11y-audit.mjs`) هر دو جهت PASS/FAIL اثبات شده.
3. **`operationId`** — ۵۸ یکتا و camelCase؛ قاعدهٔ spectral به `error` رفت و با یک
   operationId عمداً تکراری اثبات شد که دروازه می‌گیرد.
4. **audit هفت دستهٔ IAM §۷** — بازپرداخت، تغییر موجودی، تغییر وضعیت سفارش، قیمت فروش،
   هزینهٔ خرید، تأیید تأمین، وضعیت ارسال. هر چهار تست، خطای auditor را برمی‌گردانند.
5. **پوشش SQL** — `scripts/audit-pg-coverage.py`: ۸۹ از ۸۹ متد SQL تست PG دارند.

### باز (به ترتیب اولویت)

1. **پوشش تست به آستانهٔ R-CI §۳** (≥۹۰٪ مالی/موجودی، ≥۷۵٪ سایر). اندازه‌گیری واقعی
   روی schema با تست PG:

   ```
   audit=0.0  shared=0.0  ecosystem=40.0  content=45.4  inventory=47.2
   media=47.4  identity=51.2  catalog=59.3  suppliers=60.2  fulfillment=60.8
   pricing=61.7  commerce=68.3  authorization=76.1  notifications=83.3  worker=86.5
   ```

   مالی/موجودی زیر آستانه: **inventory ۴۷.۲، pricing ۶۱.۷، commerce ۶۸.۳**.
   گام coverage در CI فعلاً `continue-on-error` است؛ **blocking کردن آن فقط بعد از
   رسیدن به آستانه مجاز است** و آستانه‌ها نباید پایین بیایند.

2. **`audit` و `shared` صفر درصد** — `audit` خودِ ماژول audit است (نوشتن رکورد) و
   `shared` توابع کمکی؛ هر دو فقط از طریق بسته‌های دیگر پوشش داده می‌شوند و تست مستقیم
   ندارند. باید یا تست مستقیم بگیرند یا در گزارش coverage از مخرج خارج شوند (تصمیم).

3. **چهار کامپوننت بی‌استفادهٔ OpenAPI** — `AcceptLanguage`، `SessionInfo`،
   `RefundStatus`، `ArticleStatus`. انتخاب بین «وصل کردن» و «حذف» **تصمیم مالک** است.

4. **اختلاف مسیر با R-07** — قرارداد `/catalog/products/{slug}` می‌خواهد، کد
   `/api/v1/products/{id}` هم دارد؛ کدام مرجع است، تصمیم مالک.

5. **مسیر بوت OIDC پنل** — سمت سرور تأییدشده (کوکی HttpOnly + PKCE + state)؛ سمت مرورگر
   در Lightpanda قفل می‌کند. سه تلاش مستند شده و شاهدها نشان می‌دهند محدودیت daemon
   مرورگر است، نه باگ اپ — ولی **تأیید نهایی روی مرورگر واقعی انجام نشده**.

6. **فرم کامل محصول در پنل** — ویرایش و انتشار هست؛ ساخت محصول جدید از UI نیست.

7. **`PROJECT_MANIFEST.md`** — پس از این بازنویسی هم‌تراز شد؛ با هر تغییر وضعیت باید
   به‌روز شود.

## ۶. بازتولید کامل تأیید

```bash
cd /home/dlp/Projects/KHABAVAR/khabavar

# ۱) استاتیک
go build ./... && go vet ./... && gofmt -l .    # همه باید پاک باشند

# ۲) واحد (بدون دیتابیس)
go test ./...

# ۳) یک کلاستر PG موقت
initdb -D /tmp/pgdata -U postgres --auth=trust
pg_ctl -D /tmp/pgdata -o "-p 55432 -k /tmp" -l /tmp/pg.log start
createdb -h 127.0.0.1 -p 55432 -U postgres khabavar_test
createdb -h 127.0.0.1 -p 55432 -U postgres khabavar

# ۴) انتگرال
KH_TEST_PG_DSN="postgres://postgres@127.0.0.1:55432/khabavar_test?sslmode=disable" \
  go test -count=1 ./internal/...

# ۵) E2E دستی
for f in migrations/*.up.sql; do psql -h 127.0.0.1 -p 55432 -U postgres -d khabavar -q -f "$f"; done
psql -h 127.0.0.1 -p 55432 -U postgres -d khabavar -q -f seed/001_seed_baseline.sql
KH_ROLE=api KH_DB_URL="postgres://postgres@127.0.0.1:55432/khabavar?sslmode=disable" \
  KH_TOKEN_SIGNING_KEY=dev-key KH_HTTP_ADDR=:18080 ./bin/khabavar
# سپس جریان سفارش→پرداخت→بازپرداخت در §۴.۵
```

---

## ۷. سجل کامیت‌ها

| کامیت | موضوع |
|-------|-------|
| `144bcfb` | `chore`: ادغام `.gitignore` |
| `1e9b3cc` | `docs`: اسناد R-01..R-CI + ۱۱ سند پایه + ۴ ران‌بوک |
| `a1c4b25` | `feat(catalog)`: درخت دسته، محصول/Variant، دروازهٔ انتشار |
| `0e40078` | `feat(pricing)`: هزینهٔ ۸ جزئی، قیمت با دروازهٔ تأیید، اقتصاد واحد |
| `cd05c3e` | `feat(commerce)`: سفارش، تأیید پرداخت، بازپرداخت، تطبیق |
| `b0bf7de` | `feat(suppliers)`: چرخهٔ عمر، چندمنبعی، ارزیابی |
| `579b12d` | `feat(inventory)`: انبار، ledger، رزرو |
| `2b2a85b` | `feat(fulfillment)`: Fulfillment، مرسوله، تایم‌لاین رویداد |
| `bf1497d` | `feat(media)`: آداپتور Storage محلی |
| `905c226` | `feat(content)`: گردش کار تحریریه، دروازهٔ انتشار |
| `fbfeb92` | `feat(notifications)`: آداپتور SMS/Email + dispatcher |
| `2bad5e0` | `feat(ecosystem)`: جستجوی یکپارچه + ناوبری مشترک |
| `9cd109c` | `feat(platform/security)`: هدرها، CORS، HSTS، CSP |
| `ff717a6` | `feat(cmd)`: سیم‌کشی ماژول‌ها در unit api |
| `83a7a26` | `deploy`: prometheus + backup/restore |
| `ec87b29` | `docs`: PROJECT_MANIFEST |
| `7652e1b` | `style`: gofmt کل درخت (۳۱ فایل) |
| `0a14983` | `feat(rbac)`: احراز هویت روت‌های ادمین + outbox lease + ایزولاسیون تست |
| `d2f5b6e` | `fix(commerce)`: هم‌ترازی refunds با اسکیما + رفع تداخل روت + نشت خطا |
| `ffbe420` | `feat(commerce)`: ثبت payment_attempts تا سفارش پرداخت‌شده قابل بازپرداخت باشد |
