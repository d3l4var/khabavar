// Package catalog implements the catalog module (SAD §۴.۲, DB v2 §۶, R-04): brands,
// categories (tree), products, sellable variants, and the structured attribute
// dictionary.
//
// مسئولیت: محصول، Variant، دسته‌بندی، ویژگی‌ها، برند
// موجودیت‌ها: products, product_variants, categories, attribute_*, product_media, brands, media_assets
// وابستگی مجاز (SAD §۴.۲): media, shared
//
// قواعد وابستگی (الزامی، SAD §۴.۲):
//  1. هیچ ماژولی جدول ماژول دیگر را مستقیم UPDATE/INSERT نمی‌کند.
//  2. ارتباط بین‌ماژولی فقط از طریق API داخلی (متد در مرز ماژول) + رویداد Outbox.
//  3. وابستگی دایره‌ای ممنوع.
//  4. هر عملیات حساس: Idempotency Key + Audit (ADR-ARCH-008/010).
//
// نقطه شروع پیاده‌سازی: IMPLEMENTATION_GUIDE.md فاز مربوطه. نوع‌ها، Store و Service
// در catalog.go و catalog_db.go تعریف شده‌اند.
package catalog
