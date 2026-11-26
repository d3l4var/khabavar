KHABAVAR — مسیر مطالعه اسناد و مرور محتوا v1.1
Document ID: KH-INDEX-001
Version: 1.1 — Resolution Record (تصمیمات مالک ۲۰۲۶-۱۱-۲۵ ثبت و اجرا شد — §۷)
Date: 2026-11-25
مبنای نسخه ۱: مرور کامل محتوای هر ۱۱ سند پایه (upload/khabavar/) و هر سند فاز تولیدشده (drafts/ و download/) توسط دو عامل کاوش مستقل؛ راستی‌آزمایی تطبیق بین‌سندی پایه↔فاز روی نقش‌ها، وضعیت‌ها، ماژول‌ها، رنگ‌ها، نسخه‌ها و شناسه‌ها.
هدف این نسخه: (۱) تعیین مسیر مطالعه یکپارچه با شماره‌گذاری R-01..R-19 که اسناد پایه و خروجی‌های فاز را به ترتیب منطقی در کنار هم می‌چیند؛ (۲) ثبت یافته‌های مرور محتوا و مغایرت‌های بین‌سندی با اولویت‌بندی برای اصلاح؛ (۳) ارائه manifest تغییر نام Drive برای هم‌رسانی نام‌گذاری همه اسناد؛ (۴) ثبت سجل تصمیم‌های مالک ۲۰۲۶-۱۱-۲۵ و اجرای آن‌ها (§۷).

۱. چگونه بخوانیم
* مسیر مطالعه به ترتیب R-01 تا R-19 طراحی شده است: از سند مادر و تحلیل کسب‌وکار آغاز می‌شود، با PRD به نیازمندی‌های قابل تست تبدیل می‌شود، سپس هر دامنه (کاتالوگ، تأمین، تجارت، برند، هویت، داده، معماری) به‌ترتیب با سند پایه و سپس سند فازِ مشتق‌شده از آن دیده می‌شود، و در پایان با استراتژی آزمون، استقرار و نقشه راه جمع می‌بندد.
* هر سند پایه (B) منبع مفهومی است و سند فاز (P) اجرایی‌شده یا مشتق‌شده از آن؛ ستون «مشتق از» نشان می‌دهد کدام سند فاز روی کدام سند پایه ساخته شده تا زنجیره وابستگی شفاف بماند و خواننده بداند کدام سند مرجع نهایی است.
* شناسه‌های سند (KH-xxx-001) برای اسناد پایه‌ای که فاقد ID بودند به‌صورت بازگشتی تخصیص یافته‌اند تا هر سند در مسیر قابل ردیابی باشد؛ این تخصیص و حل دو تصادم شناسه در §۴ مستند شده است.

۲. مسیر مطالعه (R-01 تا R-19)
Table: مسیر | سند (نوع · شناسه · نسخه/وضعیت) | مشتق از | در یک نگاه
R-01 | Master Project State · پایه · KH-MGMT-001 · v1.0 Active | — | سند مادر: دامنه خواب، تأمین MIXED، تصمیم‌های معماری، نقشه راه ۹ فازی
R-02 | P1 Business Analysis & Tech Spec · پایه · KH-BIZ-001 · v1.1 | — | تحلیل کسب‌وکار + مشخصات فنی P1 (۱۲ تحویل‌داده، ۶ دسته، تأمین ترکیبی)
R-03 | Product Requirements Document (PRD) · فاز · KH-PRD-001 · v1.0 مصوب | R-02 | نیازمندی‌های قابل تست مدل A (MA-1..9، BR-01..12، UJ-01..06، O-01..10)
R-04 | Product Catalog v1.0 · پایه · KH-CAT-001 · v1.0 Draft | — | ۶ دسته، SKU، Variant، ۷ وضعیت موجودی، QC پنج‌مرحله‌ای، چرخه عمر محصول
R-05 | Supplier Evaluation & Unit Economics · پایه · KH-SUP-001 · v1.0 Draft | — | ماتریس ارزیابی وزنی (۱۰۰)، اقتصاد واحد، Hard Gates، Contribution Margin
R-06 | Commerce, Order & Fulfillment Spec · پایه · KH-COM-001 · v1.0 Draft | — | ADR-COM-001/002، ۱۷ وضعیت سفارش، ۸ وضعیت پرداخت، تفکیک Fulfillment، Outbox
R-07 | API Contract (OpenAPI KH-API-001) · فاز · KH-API-001 · v1.1.0 | R-04/05/06 | ۵۸ عملیات REST (افزودن مرجوعی پس‌تحویل)، OIDC+PKCE، ماشین‌های وضعیت، Idempotency، Money/IRR
R-08 | Brand & Design System · پایه · KH-BRAND-001 · v1.0 | — | شخصیت برند، پالت ۱۰رنگ، فونت‌های کاندید، مؤلفه‌های مشترک
R-09 | Design System v1.0 + Design Tokens · فاز · KH-DS-001 · v1.0 Draft | R-08 | توکن‌های اجرایی با کنتراست WCAG AA اندازه‌گیری‌شده، Vazirmatn نهایی، ۹ Badge
R-10 | Central Identity & Access Management (CIAM) · پایه · KH-IAM-001 · v1.0 | — | OIDC+PKCE، OTP، ۸ نقش اولیه، نشست، جداسازی پروفایل مشتری/تجارت
R-11 | IAM Matrix · فاز · KH-IAM-MATRIX-001 · v1.1 مصوب | R-10 | ۱۰ نقش، ۲۴ مجوز، ماتریس تخصیص از Seed، ۴ کلاینت OIDC، سیاست OTP/نشست
R-12 | Database Architecture v1.0 · پایه · KH-TECH-DB-001 · v1.0 [منسوخ توسط R-13] | — | طراحی مفهومی ۶ اسکیمای اولیه (بدون ماژول‌های supplier/inventory/pricing جدا)
R-13 | Database Architecture & Domain Design v2.0 · پایه · KH-TECH-DB-001 · v2.0 Draft | R-12 | ۹ ماژول، ۵۲ جدول، قیدهای CHECK، ۱۰ تراکنش بحرانی، ۴ نقش DB، ۶ ADR
R-14 | DB Migrations v2.0 + Seed Baseline · فاز · KH-DB-MIG-001 · v2.0 | R-13 | ۱۰ مهاجرت (000001-000010) + Seed (۱۰ نقش، ۲۴ مجوز، ۱۱ موضوع، ۴ کلاینت)
R-15 | Software Architecture Document (SAD) v1.0 · پایه · KH-SAD-001 · v1.0 Draft | — | چشم‌انداز معماری، ۱۰ اصل، ۳ شخصیت بصری، پنل مدیریت، ۱۰ فاز توسعه
R-16 | SAD v2.1 · فاز · KH-SAD-001 · v2.1 مصوب | R-15 | ۱۳ ماژول با مرز صریح، ADR Registry (۲۳ ADR)، ۶ جریان کلیدی، Adapter/Mock، استقرار §12
R-17 | Test Strategy v1.1 · فاز · KH-TS-001 · v1.1 Draft | R-03/07/13 | هرم چهارلایه، ۶ سناریوی هم‌روندی، امنیت، معیارهای عملکرد عددی، UAT/Go-Live
R-18 | Deployment Spec & Runbook v1.1 · فاز · KH-DEP-001 · v1.1 Draft | R-16 | توپولوژی VPS، systemd با KH_ROLE، نقش‌های DB، بکاپ سه‌لایه، Go-Live gate
R-19 | Implementation Roadmap v1.0 · پایه · KH-ROAD-001 · v1.0 | — | ۱۰ فاز دروازه‌دار P1-P10، Mock-first، سه محیط، چک‌لیست تحویل هر فاز

۳. مرور محتوا و تطبیق بین‌سندی
مرور دو عامل کاوش نشان می‌دهد ساختار سندی پروژه منسجم است: مهاجرت‌ها پیاده‌سازی وفادار DB v2 (R-13) هستند، ۹ وضعیت سفارش در سه سند (Design System، tokens.json، Migration 000006) هم‌خوان است، و PRD بازنویسی قابل‌آزمون Commerce (R-06) است بدون افزودن تصمیم تجاری جدید. با این حال، چالش اصلی این است که اسناد فاز در چند مورد از حرف اسناد پایه فاصله گرفته‌اند — گاهی به‌عنوان بازطراحی آگاهانه (مثل مدل Intent در OpenAPI) و گاهی به‌عنوان مغایرت واقعی که باید اصلاح شود. این بخش یافته‌ها را در سه دسته مثبت، مغایرت و شکاف ارائه می‌دهد و §۴ آن‌ها را به‌صورت جدول اولویت‌بندی‌شده جمع می‌بندد.

۳.۱ تطبیق‌های تأییدشده
* PRD (R-03) ↔ Commerce (R-06): تطابق کامل روی مدل A، یک‌پرداخت (ADR-COM-001)، تفکیک بسته، Idempotency، Snapshot قیمت/مشخصات، و بازپرداخت جزئی با سقف.
* OpenAPI (R-07) ↔ Commerce/PRD روی مکانیک‌های اصلی: یک Payment Intent، تفکیک Fulfillment، هدر Idempotency، Outbox تراکنشی، Snapshot، سقف بازپرداخت با 409.
* ۹ وضعیت سفارش سه‌سندی هم‌خوان: Design System §۶ = tokens.json orderStatusBadge = Migration 000006 CHECK (همان نه واژه).
* مهاجرت‌ها (R-14) پیاده‌سازی وفادار DB v2 (R-13): همه قیدهای CHECK کلیدی، نقش‌های DB (db_migrator/app_runtime/reporting_reader/backup_operator)، و تراکنش‌های بحرانی §۱۴.
* تاریخ و وابستگی: هر ۷ سند فاز یک تاریخ ۲۰۲۶-۱۱-۲۴ دارند و ارجاع‌های متقابل آن‌ها به SAD v2 و DB v2 و CIAM درست است.

۳.۲ شکاف‌های تحویل (نه مغایرت محتوا، بلکه نبود در Drive)
* OpenAPI (R-07)، DB Migrations (R-14) و Seed Baseline هنوز در Google Drive منتشر نشده‌اند، با اینکه سه سند فاز دیگر به OpenAPI ارجاع داده‌اند (IAM v1، TS v1 «۵۷ عملیات»، SAD v2 ADR-API-001). بارگذاری آن‌ها شرط تکمیل P3 طبق Roadmap است؛ بستهٔ آپلود در scripts/drive_prep آماده است و به‌محض دردسترس‌بودن ابزار Drive در یک pass اجرا می‌شود.

۴. جدول اولویت اصلاح مغایرت‌ها
Table: اولویت | مغایرت/شکاف | سندهای درگیر | اقدام پیشنهادی
1 | تصادم شناسه KH-IAM-001 — CIAM و IAM Matrix هر دو همین ID را دارند | CIAM (R-10) + IAM Matrix (R-11) | تغییر شناسه ماتریس به KH-IAM-MATRIX-001 یا ادغام در CIAM
2 | نقش SUPPLIER_OPERATOR حذف‌شده از CIAM بدون جایگزین — supplier.manage/evaluate فقط روی SUPER_ADMIN مانده | CIAM (R-10) + IAM Matrix (R-11) + Seed | بازگرداندن نقش یا تخصیص مجوزها به STORE_MANAGER
3 | SAD v2 §3 نام‌های قدیمی نقش‌ها (ADMIN/SUPPORT/CONTENT_EDITOR/SUPPLIER_OPERATOR) به‌جای ۹ نقش مصوب | SAD v2 (R-16) vs IAM Matrix (R-11) | به‌روزرسانی §3 با ۹ نقش مصوب
4 | DEP v1 §3 واحد جدا khabavar-identity.service در تضاد با تک‌باینری SAD v2 §4 (ADR-DB-002) | DEP v1 (R-18) vs SAD v2 (R-16) | شفاف‌سازی: همان باینری با flag یا ادغام واحد
5 | MIXED SupplyType در Catalog + ۵ SKU ولی غایب در OpenAPI enum | Catalog (R-04) vs OpenAPI (R-07) | افزودن MIXED به enum یا حذف از کاتالوگ و بازبرچسب SKUها
6 | چرخه عمر تأمین‌کننده: ۸ وضعیت در Supplier Eval vs ۴ در OpenAPI (+PROBATION/RETIRED بی‌سابقه) | Supplier Eval (R-05) vs OpenAPI (R-07) | تطبیق enum وضعیت
7 | endpoint درخواست مرجوعی مشتری غایب در OpenAPI (Commerce §9 + PRD SH-M09 Must) | Commerce (R-06) + PRD (R-03) vs OpenAPI (R-07) | افزودن POST /orders/{id}/return-requests
8 | وضعیت‌های موجودی مشتری: ۷ در Catalog vs ۳ در OpenAPI (SUPPLIER_AVAILABLE/PRE_ORDER/PENDING_VERIFICATION حذف) | Catalog (R-04) vs OpenAPI (R-07) | بازگرداندن سه وضعیت به availability
9 | ماشین‌های وضعیت فشرده‌شده Order/Fulfillment/Shipment (17/13/10 → 9/8/6) — وضعیت‌های PARTIALLY_CANCELLED/REFUND_PENDING از Order حذف | Commerce (R-06) vs OpenAPI (R-07) | back-annotation در Commerce یا بازگرداندن وضعیت‌ها
10 | چرخش Refresh Token در IAM v1 الزامی ولی Migration 000001 ستون ندارد | IAM Matrix (R-11) vs Migration 000001 (R-14) | افزودن ستون rotation یا توکن جدا
11 | editorial_reviews در SAD v2 §4.1 ولی غایب در DB v2 §12 و Migration 000008 | SAD v2 (R-16) vs DB v2 (R-13) + Migration 000008 | افزودن جدول یا حذف ارجاع از SAD
12 | سقف تخصیص Fulfillment (08 §11.2) بدون CHECK و بدون سناریوی آزمون صریح | DB v2 (R-13) + Migration 000007 + TS v1 (R-17) | افزودن CHECK یا سناریوی TS
13 | QC زیروضعیت‌های محصول (SUPPLIER_REVIEW/SAMPLE_REQUESTED/QC_PENDING) غایب در AdminProductStatusInput | Catalog (R-04) vs OpenAPI (R-07) | بازگرداندن زیروضعیت‌ها به enum
14 | Hard Gates تأمین‌کننده در OpenAPI رمزگذاری‌نشده (فقط چک سمت سرور) | Supplier Eval (R-05) vs OpenAPI (R-07) | افزودن endpoint یا قید قابل‌مشاهده
15 | Master §5 و Roadmap §9 وضعیت راکد (می‌گویند PRD/OpenAPI/SAD «نیازمند تکمیل») | Master (R-01) + Roadmap (R-19) | به‌روزرسانی وضعیت اسناد به مصوب/منتشرشده
16 | مقیاس امتیازدهی تأمین‌کننده ۱-۵→۱۰۰ vs ۰-۱۰۰ مستقیم در OpenAPI | Supplier Eval (R-05) vs OpenAPI (R-07) | تطبیق مقیاس و نام‌گذاری (availability ↔ پایداری تأمین)
17 | شمارش موضوعات مجله ۱۱ vs ۱۲ — تصمیم مالک باز | Seed (R-14) vs PRD MG-M02 (R-03) | تصمیم مالک: موضوع دوازدهم یا اصلاح PRD به ۱۱
18 | Migration 000010 با هدر «اختیاری» در تضاد با وابستگی سخت DEP v1 به نقش‌هایش | Migration 000010 (R-14) vs DEP v1 (R-18) | حذف برچسب «اختیاری» و افزودن به طرح 08 §16
19 | ارجاع .md به‌جای .txt در Supplier Eval و Commerce + نقص توضیح payment_provider (copy-paste) در OpenAPI | R-05، R-06، R-07 | اصلاح آرایشی و توضیح فیلد
20 | OpenAPI/Migrations/Seed هنوز در Drive منتشر نشده‌اند | R-07، R-14 | بارگذاری وقتی پلاگین Drive فعال شود

۵. manifest تغییر نام Drive (وقتی پلاگین فعال شود)
برای هم‌رسانی نام‌گذاری، هر سند در Drive به الگوی «KHABAVAR — R-NN · TITLE» تغییر نام می‌یابد تا شماره مسیر مطالعه در نام فایل دیده شود. همه اسناد پایه در پوشه «اسناد پایه» و همه اسناد فاز در پوشه «Docs» قرار می‌گیرند. سه سند باقی‌مانده (OpenAPI، Migrations، Seed) در pass عملیاتی Drive بارگذاری و نام‌گذاری می‌شوند (بستهٔ آماده: scripts/drive_prep).
Table: نام فعلی در Drive | نام هدف (R-numbered)
KHABAVAR — MASTER PROJECT STATE | KHABAVAR — R-01 · Master Project State
KHABAVAR — P1 Business Analysis & Technical Specification | KHABAVAR — R-02 · P1 Business Analysis & Tech Spec
KHABAVAR — PRD v1 | KHABAVAR — R-03 · PRD v1.0
KHABAVAR — Product Catalog v1 | KHABAVAR — R-04 · Product Catalog v1.0
KHABAVAR — Supplier Evaluation & Unit Economics | KHABAVAR — R-05 · Supplier Evaluation & Unit Economics
KHABAVAR — Commerce, Order & Fulfillment Specification | KHABAVAR — R-06 · Commerce, Order & Fulfillment Spec
(هنوز آپلود نشده) | KHABAVAR — R-07 · API Contract (OpenAPI)
KHABAVAR — BRAND & DESIGN SYSTEM | KHABAVAR — R-08 · Brand & Design System
KHABAVAR — Design System v1.0 + Tokens | KHABAVAR — R-09 · Design System v1.0 + Tokens
KHABAVAR — Central Identity & Access Management | KHABAVAR — R-10 · CIAM v1.0
KHABAVAR — IAM v1.0 | KHABAVAR — R-11 · IAM Matrix v1.1
KHABAVAR — DATABASE ARCHITECTURE | KHABAVAR — R-12 · Database Architecture v1.0 [superseded]
KHABAVAR — Database Architecture & Domain Design | KHABAVAR — R-13 · Database Architecture & Domain Design v2.0
(هنوز آپلود نشده) | KHABAVAR — R-14 · DB Migrations v2.0 + Seed
KHABAVAR - SAD | KHABAVAR — R-15 · SAD v1.0
KHABAVAR — SAD v2.0 | KHABAVAR — R-16 · SAD v2.1
KHABAVAR — Test Strategy v1.0 | KHABAVAR — R-17 · Test Strategy v1.1
KHABAVAR — Deployment Spec v1.0 | KHABAVAR — R-18 · Deployment Spec v1.1
KHABAVAR — IMPLEMENTATION ROADMAP | KHABAVAR — R-19 · Implementation Roadmap v1.0

۶. توصیه مسیر اصلاح
* اولویت ۱ تا ۸ (شدت بالا): در ۲۰۲۶-۱۱-۲۵ با تصمیم مالک حل شدند — سجل و محل اجرا در §۷؛ زمان‌بندی پیش از شروع پیاده‌سازی کد حفظ شد چون مستقیماً روی قرارداد API، مدل داده و کنترل دسترسی اثر می‌گذاشتند.
* اولویت ۹ تا ۱۴ (متوسط): در همان فاز پیاده‌سازی ماژول مربوطه حل می‌شوند؛ هر کدام به یک سند فاز و یک سند پایه گره خورده‌اند و اصلاحشان به‌صورت جفت انجام می‌شود.
* اولویت ۱۵ تا ۲۰ (پایین/تحویل): آپلود سه سند باقی‌مانده و به‌روزرسانی وضعیت اسناد راکد، در pass عملیاتی Drive با بستهٔ آماده scripts/drive_prep انجام می‌شود.

۷. سجل تصمیم‌های مالک — اجرای هشت مغایرت اولویت‌بالا (۲۰۲۶-۱۱-۲۵)
Table: مغایرت (اولویت §۴) | تصمیم مالک | اجرا در
1 | تصادم شناسه KH-IAM-001 | انجام شد — شناسه ماتریس به KH-IAM-MATRIX-001 تغییر کرد؛ CIAM همان KH-IAM-001 را نگه می‌دارد | IAM Matrix v1.1 + این سند
2 | نقش SUPPLIER_OPERATOR | نقش داده شد — بازگشت به‌عنوان نقش دهم با supplier.manage/evaluate | Seed v1.1 + IAM Matrix v1.1 §۲/§۴
3 | MIXED SupplyType | به‌ازای سفارش — enum دومقداری در سطح Variant حفظ شد؛ MIXED استراتژی سطح محصول در Catalog و منبع Fulfillment به‌ازای هر سفارش تصمیم‌گیری می‌شود | OpenAPI v1.1.0 (یادداشت SupplyType)
4 | چرخه عمر تأمین‌کننده | هشت وضعیت مصوب بازگشت + PROBATION نگه‌داری‌شده در این نسخه با قابلیت حذف؛ RETIRED حذف (≈ INACTIVE) | OpenAPI v1.1.0 (enum نُه‌مقداری)
5 | endpoint مرجوعی مشتری | انجام شد — POST /orders/{orderNumber}/return-requests با قلم‌ها در بدنه و Idempotency-Key | OpenAPI v1.1.0
6 | وضعیت‌های موجودی مشتری | تأیید — شش وضعیت مشتری‌روی؛ DISCONTINUED در وضعیت محصول باقی ماند | OpenAPI v1.1.0 (CartItem.availability)
7 | واحد identity جدا | انجام شد — هر سه unit systemd از باینری واحد با متغیر نقش KH_ROLE اجرا می‌شوند | DEP v1.1 §۳
8 | SAD §۳ نقش‌های قدیمی | انجام شد — ارجاع به ۱۰ نقش مصوب IAM Matrix به‌جای تکرار فهرست | SAD v2.1 §۳
همه قواعد Spectral معلق‌شده فعال شدند، ci.yml به شمارش ۱۰ نقش به‌روزرسانی شد و دروازه‌های CI اکنون روی قرارداد v1.1.0 می‌ایستند. اولویت‌های ۹ تا ۱۴ طبق §۶ در فاز پیاده‌سازی ماژول مربوطه حل می‌شوند.
