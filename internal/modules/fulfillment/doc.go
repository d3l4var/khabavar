// Package fulfillment — ماژول «fulfillment».
//
// مسئولیت: تفکیک Fulfillment، مرسوله‌ها، رویدادهای حمل
// موجودیت‌ها: fulfillment_orders, fulfillment_items, shipments, shipment_items, shipment_events
// وابستگی مجاز (SAD §۴.۲): commerce, inventory, suppliers
//
// قواعد وابستگی (الزامی، SAD §۴.۲):
//  ۱. هیچ ماژولی جدول ماژول دیگر را مستقیم UPDATE/INSERT نمی‌کند.
//  ۲. ارتباط بین‌ماژولی فقط از طریق API داخلی (متد در مرز ماژول) + رویداد Outbox.
//  ۳. وابستگی دایره‌ای ممنوع.
//  ۴. هر عملیات حساس: Idempotency Key + Audit (ADR-ARCH-008/010).
//
// نقطه شروع پیاده‌سازی: IMPLEMENTATION_GUIDE.md فاز مربوطه.
package fulfillment

// Service مرز ماژول است؛ فراخوانی‌کننده فقط از طریق این رابط با ماژول صحبت می‌کند.
type Service interface {
	// TODO: متدهای دامنه طبق OpenAPI و SAD §۷
}

// New سازنده پیش‌فرض؛ وابستگی‌ها تزریق می‌شوند (هرگز import مستقیم از ماژول غیرمجاز).
func New( /* deps */ ) Service {
	panic("not implemented — see IMPLEMENTATION_GUIDE.md")
}
