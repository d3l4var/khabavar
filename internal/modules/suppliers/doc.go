// Package suppliers implements the suppliers module (SAD §۴.۲, DB v2 §۷, R-05):
// suppliers, supplier_products, supplier_variants (multi-source supply offers with
// price validity), price history, and weighted evaluations.
//
// مسئولیت: تأمین‌کنندگان، ارزیابی وزنی، قیمت/موجودی تأمین با تاریخ اعتبار
// موجودیت‌ها: suppliers, supplier_products, supplier_variants,
//
//	supplier_variant_price_history, supplier_evaluations
//
// وابستگی مجاز (SAD §۴.۲): catalog (فقط id), shared
//
// قواعد وابستگی (الزامی، SAD §۴.۲):
//
//	۱. هیچ ماژولی جدول ماژول دیگر را مستقیم UPDATE/INSERT نمی‌کند.
//	۲. ارتباط بین‌ماژولی فقط از طریق API داخلی (متد در مرز ماژول) + رویداد Outbox.
//	۳. وابستگی دایره‌ای ممنوع.
//	۴. هر عملیات حساس: Idempotency Key + Audit (ADR-ARCH-008/010).
//
// نقطه شروع پیاده‌سازی: IMPLEMENTATION_GUIDE.md فاز مربوطه. نوع‌ها، Store و Service
// در suppliers.go و suppliers_db.go تعریف شده‌اند.
package suppliers
