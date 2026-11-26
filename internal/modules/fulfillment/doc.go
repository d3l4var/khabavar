// Package fulfillment implements the fulfillment module (SAD §۴.۲, DB v2 §۱۱,
// R-06 §۱۱, ADR-COM-001): fulfillment orders (per-source-type), fulfillment items,
// shipments (multiple per fulfillment), shipment items (split across packages),
// and the append-only shipment event timeline.
//
// مسئولیت: تفکیک Fulfillment، مرسوله‌ها، رویدادهای حمل
// موجودیت‌ها: fulfillment_orders, fulfillment_items, shipments, shipment_items, shipment_events
// وابستگی مجاز (SAD §۴.۲): commerce (فقط id), inventory (فقط id), suppliers (فقط id), shared
//
// قواعد وابستگی (الزامی، SAD §۴.۲):
//  ۱. هیچ ماژولی جدول ماژول دیگر را مستقیم UPDATE/INSERT نمی‌کند.
//  ۲. ارتباط بین‌ماژولی فقط از طریق API داخلی (متد در مرز ماژول) + رویداد Outbox.
//  ۳. وابستگی دایره‌ای ممنوع.
//  ۴. هر عملیات حساس: Idempotency Key + Audit (ADR-ARCH-008/010).
//
// نقطه شروع پیاده‌سازی: IMPLEMENTATION_GUIDE.md فاز مربوطه. نوع‌ها، Store و Service
// در fulfillment.go و fulfillment_db.go تعریف شده‌اند.
package fulfillment
