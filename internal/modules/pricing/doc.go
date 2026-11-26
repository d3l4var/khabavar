// Package pricing implements the pricing module (SAD §۴.۲, DB v2 §۹, R-05): landed
// cost of goods, approved selling prices, per-variant unit economics, and the order
// financial snapshot basis.
//
// مسئولیت: هزینه تمام‌شده، قیمت فروش، سود مشارکت، Snapshot مالی
// موجودیت‌ها: product_cost_records, product_price_records, order_financial_snapshots
// وابستگی مجاز (SAD §۴.۲): catalog, suppliers, shared
//
// قواعد وابستگی (الزامی، SAD §۴.۲):
//  1. هیچ ماژولی جدول ماژول دیگر را مستقیم UPDATE/INSERT نمی‌کند.
//  2. ارتباط بین‌ماژولی فقط از طریق API داخلی (متد در مرز ماژول) + رویداد Outbox.
//  3. وابستگی دایره‌ای ممنوع.
//  4. هر عملیات حساس: Idempotency Key + Audit (ADR-ARCH-008/010).
//
// نقطه شروع پیاده‌سازی: IMPLEMENTATION_GUIDE.md فاز مربوطه. نوع‌ها، Store و Service
// در pricing.go و pricing_db.go تعریف شده‌اند.
package pricing
