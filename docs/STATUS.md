# وضعیت KHABAVAR — سند تحویل

> این سند **فقط چیزهایی را می‌گوید که با اجرای واقعی دستور اثبات شده‌اند**. هر عدد
> خروجی یک فرمان است، نه برآورد. بندهای باز صریحاً به‌عنوان باز علامت‌گذاری شده‌اند.

**شاخه:** `main` · **HEAD:** `5b5cee7` · **کامیت‌ها:** ۱۳۱ · **تاریخ:** ۲۰۲۶-۱۱-۲۷

---

## ۱. اجزای محصول

| جزء | وضعیت | توضیح |
|---|---|---|
| بک‌اند Go (تک‌باینری) | ✅ کامل | `KH_ROLE=identity\|api\|worker` |
| API عمومی + پنل | ✅ کامل | ۱۱۵ مسیر (path) / ۱۲۶ عملیات — کد و قرارداد **دقیقاً منطبق** |
| چهار اپ وب | ✅ بیلد سبز | `main`, `shop`, `admin`, `magazine` (fa-IR/RTL) |
| پایگاه داده | ✅ ۱۶ migration | forward-only، PostgreSQL 16 |
| Storybook | ✅ | کامپوننت‌های مشترک |

**کد:** ۱۶٬۷۱۲ خط Go تولیدی · ۱۸٬۳۰۴ خط تست · **۳۹۲ تست**

---

## ۲. گیت‌های CI — همه blocking و همه آزموده

هر گیت با یک **قرمزِ عمدی** آزموده شده که واقعاً می‌گیرد:

| گیت | فرمان | اثبات |
|---|---|---|
| ساخت و ایستا | `go build ./... && go vet ./... && gofmt -l .` | ✅ خروجی خالی |
| تست‌ها | `go test ./...` (با و بدون PG) | ✅ ۱۸ بسته ok، ۰ FAIL |
| مسیرها ↔ قرارداد | `audit-routes.py --fail --fail-undocumented` | ✅ واگرایی تزریقی → exit 1 |
| انطباق قرارداد | `audit-contract-conformance.py --fail` | ✅ واگرایی تزریقی → exit 1 |
| پوشش SQL | `audit-pg-coverage.py --fail-under 95` | ✅ ۹۲/۹۲ |
| آستانهٔ پوشش | `audit-coverage.py --fail` | ✅ ۱۵/۱۵ |
| سطح env | `audit-env.py --fail-on-missing` | ✅ |
| OpenAPI | `spectral lint` | ✅ **۰ خطا، ۰ هشدار** |

**دو گیت advisory که قابل تأیید نبودند** — و صریح می‌گویم چرا:

- **`golangci-lint`** — اجرای محلی شکست خورد:
  `could not import sync/atomic ... export data version 4 is greater than maximum supported version 2`
  ناسازگاری ابزار با toolchain است، نه باگ کد. **روشن کردن گیتی که سبز بودنش اثبات نشده، بدتر از خاموش گذاشتنش است.**
- **a11y (axe-core)** — نیاز به runner با مرورگر واقعی؛ محلی با `scripts/a11y-audit.mjs` دور زده شد.

---

## ۳. پوشش تست — ۱۵ از ۱۵ بالای آستانه

آستانهٔ مالی/موجودی **۹۰٪**، بقیه **۷۵٪**.

```
shared         100.0%  ✓
ecosystem       97.8%  ✓
inventory       92.0%  / 90%  ✓
pricing         90.0%  / 90%  ✓ [ENFORCED]
commerce        90.1%  / 90%  ✓
worker          86.5%  ✓
audit           85.2%  ✓
notifications   83.3%  ✓
media           79.8%  ✓
content         78.9%  ✓
suppliers       78.5%  ✓
catalog         77.2%  ✓
authorization   76.1%  ✓
identity        75.3%  ✓
fulfillment     75.0%  ✓
```

**پوشش SQL: ۹۲/۹۲ (۱۰۰٪)** — هر کوئری در کد یک تست دارد.

> نکتهٔ عملیاتی: `audit-coverage.py --fail` **بدون** `KH_TEST_PG_DSN` قیمت‌گذاری را ۴۱.۱٪
> می‌بیند و قرمز می‌شود. تست‌های DBStore فقط با پایگاه داده اجرا می‌شوند؛ CI متغیر را ست
> می‌کند. اجرای محلی بدون DSN گیت را قرمز نشان می‌دهد و این درست است.

---

## ۴. باگ‌های پیدا و رفع‌شده (۲۲ مورد)

مهم‌ترین‌ها، با اثر واقعی:

### ۴.۱ امنیتی — تأیید پرداخت بدون امضا ✅ رفع شد
```
POST /api/v1/orders/{id}/confirm-payment  →  200 {"status":"paid"}   (بدون امضا)
```
مسیر روی `api.public` بود و `ConfirmPaymentForOrder` وضعیت intent `REQUIRES_PAYMENT` را
می‌پذیرفت — **هر کسی می‌توانست سفارش خودش را PAID کند بدون پول**. حالا امضای HMAC روی
بدنهٔ خام اجباری است؛ بدون امضا → ۴۰۱ و سفارش دست‌نخورده.

### ۴.۲ نشت PII سفارش
`GET /orders/{id}` بدون بررسی مالکیت، سفارش دیگران را برمی‌گرداند. حالا ۴۰۳.

### ۴.۳ هویت از بدنه (سه مورد)
`created_by`/`actor` از بدنهٔ درخواست خوانده می‌شد — پنل می‌توانست تغییر موجودی را به نام
دیگری ثبت کند. حالا از `shared.ActorID(ctx)`؛ بدون actor → ۴۰۱.

### ۴.۴ ورود کارمند روی واحد API وجود نداشت
`POST /api/v1/auth/admin/login` فقط در واحد identity ثبت شده بود؛ در استقرار تک‌واحدی API
ورود پنل **۴۰۴** می‌داد.

### ۴.۵ خطای کلاینت → ۵۰۰ (پنج مورد)
`errors.New` خام بدون sentinel، پس `statusForError` به default می‌افتاد:
- inventory: مقدار صفر، `order_item_id` غایب، مقدار رزرو صفر
- commerce: merge سبد بدون نشست → حالا ۴۰۱
- suppliers: وضعیت ناموجود → حالا ۴۰۰ (قبلاً ۴۰۹)

### ۴.۶ نشت PascalCase در ۱۱ ساختار
`{"ID":…,"WarehouseID":…}` → `{"id":…,"warehouse_id":…}`

### ۴.۷ `ActorID(nil)` panic
`shared` صفر درصد بود؛ nil context فرآیند را می‌انداخت. حالا `""` → ۴۰۱.

### ۴.۸ گیت‌هایی که هرگز نمی‌گرفتند (سه مورد)
- `audit-contract-conformance.py` با `continue-on-error: true` **و** بدون `--fail` صدا زده
  می‌شد — دو بار بی‌اثر.
- `audit-routes.py` فقط یک جهت را می‌سنجید؛ **۶۸ مسیر مستندنشده** پیدا شد.

### ۴.۹ تست‌های سبزی که رفتار غلط را قفل کرده بودند (چهار مورد)
sentinel اشتباه در suppliers · ۴۰۹ در برابر ۴۰۰ · تأیید پرداخت بدون امضا · `PATCH` آدرس
که در عمل `PUT` بود.

---

## ۵. تصمیم‌های مالک — همه بسته شدند

| تصمیم | انتخاب | نتیجه |
|---|---|---|
| `/oauth/authorize` | کوکی HttpOnly + CSRF | ✅ |
| Storybook | انجام شود | ✅ |
| ۹ واگرایی قرارداد | ترکیب | ✅ |
| ۶۸ مسیر مستندنشده | گزینهٔ ۲: مستندسازی منتخب | ✅ ۵۸ → ۱۱۵ مسیر |
| ۴ کامپوننت OpenAPI | وصل یا حذف | ✅ `Refund`/`AdminArticleView` وصل، `AcceptLanguage`/`SessionInfo` حذف |
| `PATCH /me/addresses` | partial واقعی | ✅ |
| تأیید پرداخت بدون امضا | گزینهٔ ۱: امضای اجباری | ✅ |

---

## ۶. بندهای باز — صریح

### ۶.۱ `golangci-lint` قابل تأیید نیست
ناسازگاری export data با toolchain این محیط. نیاز به محیط با Go سازگار. **گیت advisory می‌ماند.**

### ۶.۲ a11y در runner اثبات نشده
`@axe-core/cli` نصب نمی‌شود (chromedriver گوگل مسدود). با `scripts/a11y-audit.mjs` دور زده
شد؛ اجرای CI آن تأیید نشده.

### ۶.۳ محدودیت نرخ ورود کارمند
`AdminLogin` سقف ندارد — گذرواژهٔ کارمند بی‌نهایت قابل حدس است. OTP سقف ۵/ساعت دارد.
**تصمیم سیاست امنیتی لازم است.**

### ۶.۴ `KH_PAYMENT_WEBHOOK_SECRET` در production
اگر خالی باشد `VerifyWebhookSignature` همیشه `false` است و تأیید پرداخت کاملاً بسته
می‌ماند — رفتار امن، ولی پرداخت واقعی کار نمی‌کند تا کلید تنظیم شود.

### ۶.۵ مسیر بوت OIDC پنل — تأیید مرورگری ناقص
سمت سرور تأیید شد؛ daemon مرورگر روی `?code=...` قفل می‌کند (محدودیت مرورگر، نه اپ).

### ۶.۶ ۶۷ از ۷۱ handler ادمین بدون audit
IAM §۷ فقط بازپرداخت، بخشی از کاتالوگ و موجودی/قیمت‌گذاری را پوشش می‌دهد. بقیه تریل
ندارند. **دامنهٔ IAM §۷ باید روشن شود.**

---

## ۷. فرمان‌های تأیید

```bash
# ایستا
go build ./... && go vet ./... && gofmt -l .

# تست‌ها (بدون پایگاه داده)
go test ./...

# تست‌ها (با پایگاه داده — لازم برای DBStore)
KH_TEST_PG_DSN="postgres://...@127.0.0.1:55433/khabavar_fresh?sslmode=disable" go test -count=1 ./...

# گیت‌های قرارداد
python3 scripts/audit-routes.py --fail --fail-undocumented
python3 scripts/audit-contract-conformance.py --fail
KH_TEST_PG_DSN="..." python3 scripts/audit-coverage.py --fail
python3 scripts/audit-pg-coverage.py --fail-under 95
npx @stoplight/spectral-cli lint api/openapi.yaml --ruleset api/.spectral.yaml

# اجرا
KH_ROLE=api KH_DB_URL=... KH_TOKEN_SIGNING_KEY=... KH_HTTP_ADDR=:8080 \
  KH_STORAGE_LOCAL_ROOT=/tmp/khmedia ./khabavar
```

---

## ۸. سجل کار

جزئیات هر تغییر، با شاهد و علت، در `docs/ACTIVITY-LOG.md` بندهای §۴.۱ تا §۴.۱۰۲.
