# مسیر مطالعه اسناد (R-01 → R-19 + R-CI + R-INDEX)

این ایندکس اسناد پایه و خروجی‌های فاز پروژه خواب‌آور را به ترتیب منطقی مطالعه می‌چیند. اسناد
مرجع کامل در مجموعهٔ اسناد پروژه (پوشهٔ `khabavar-drive-payloads/` یا Google Drive — پوشهٔ «اسناد
پایه» و «Docs») نگه‌داری می‌شوند؛ آن‌هایی که در خود مخزن هستند با مسیر نسبی علامت‌گذاری شده‌اند.

| مسیر | سند | نوع | شناسه | نسخه | محل |
|------|-----|-----|-------|------|-----|
| R-01 | Master Project State | پایه | KH-MGMT-001 | v1.0 | مجموعهٔ اسناد |
| R-02 | P1 Business Analysis & Tech Spec | پایه | KH-BIZ-001 | v1.1 | مجموعهٔ اسناد |
| R-03 | Product Requirements Document | فاز | KH-PRD-001 | v1.0 مصوب | مجموعهٔ اسناد |
| R-04 | Product Catalog | پایه | KH-CAT-001 | v1.0 | مجموعهٔ اسناد |
| R-05 | Supplier Evaluation & Unit Economics | پایه | KH-SUP-001 | v1.0 | مجموعهٔ اسناد |
| R-06 | Commerce, Order & Fulfillment Spec | پایه | KH-COM-001 | v1.0 | مجموعهٔ اسناد |
| R-07 | API Contract (OpenAPI) | فاز | KH-API-001 | v1.1.0 | [`api/openapi.yaml`](../api/openapi.yaml) |
| R-08 | Brand & Design System | فاز | KH-BRAND-001 | v1.0 | مجموعهٔ اسناد |
| R-09 | Design System + Design Tokens | فاز | KH-DS-001 | v1.0 | مجموعهٔ اسناد (توکن: `web/` اپ‌ها) |
| R-10 | Central Identity & Access (CIAM) | پایه | KH-CIAM-001 | v1.0 | مجموعهٔ اسناد |
| R-11 | IAM Matrix | فاز | KH-IAM-MATRIX-001 | v1.1 | مجموعهٔ اسناد (پیاده: `internal/modules/authorization`) |
| R-13 | Database Architecture | فاز | KH-DB-001 | v2.0 | مجموعهٔ اسناد |
| R-14 | DB Migrations + Seed | فاز | KH-DB-001 | v2.0 / v1.1 | [`migrations/`](../migrations/) · [`seed/`](../seed/) |
| R-16 | Software Architecture Description | فاز | KH-SAD-001 | v2.1 | مجموعهٔ اسناد |
| R-17 | Test Strategy | فاز | KH-TS-001 | v1.1 | مجموعهٔ اسناد |
| R-18 | Deployment Spec | فاز | KH-DEP-001 | v1.1 | [`deploy/`](../deploy/) (systemd + nginx) |
| R-19 | Implementation Roadmap | پایه | KH-MGMT-001 | v1.0 | مجموعهٔ اسناد |
| R-CI | CI Pipeline & Quality Gates | فاز | KH-CI-001 | v1.1 | [`.github/workflows/ci.yml`](../.github/workflows/ci.yml) |
| R-INDEX | Reading Path & Content Review | فاز | KH-INDEX-001 | v1.1 | مجموعهٔ اسناد |

## ترتیب پیشنهادی برای پیاده‌ساز

۱. **R-19** (نقشهٔ فازها) → ۲. **R-16** (معماری و مرز ماژول) → ۳. **R-07** (قرارداد API در مخزن) →
۴. **R-14** (مهاجرت و seed در مخزن) → ۵. **R-11** (نقش‌ها و مجوزها) → ۶. **R-CI** (دروازه‌های کیفیت) →
۷. **IMPLEMENTATION_GUIDE.md** (گام‌به‌گام اجرایی) → ۸. **R-18** (استقرار) → ۹. **R-17** (آزمون).

## سجل تصمیم‌های مالک ۲۰۲۶-۱۱-۲۵

هشت تصمیم مصوب در R-INDEX §۷: (۱) تغییر شناسه ماتریس به KH-IAM-MATRIX-001؛ (۲) بازگشت نقش
SUPPLIER_OPERATOR (Seed اکنون ۱۰ نقش)؛ (۳) MIXED به‌عنوان استراتژی سطح محصول با حفظ enum
دومقداری؛ (۴) چرخهٔ عمر تأمین‌کننده با هشت وضعیت مصوب + PROBATION؛ (۵) افزودن
`POST /orders/{orderNumber}/return-requests`؛ (۶) شش وضعیت مشتری‌روی موجودی؛ (۷) تک‌باینری با
متغیر نقش در DEP v1.1؛ (۸) ارجاع اکتورهای SAD §۳ به ماتریس. همه در مخزن اعمال شده‌اند.

- [STATUS.md](STATUS.md) — سند تحویل: وضعیت تأییدشده، گیت‌ها، باگ‌ها و بندهای باز
