// Package inventory implements the inventory module (SAD §۴.۲, DB v2 §۸, R-04 §۶):
// warehouses, stock balances, the append-only stock movement ledger, and
// transactional inventory reservations.
//
// مسئولیت: انبار، موجودی، Ledger حرکت، رزرو تراکنشی
// موجودیت‌ها: warehouses, stock_balances, stock_movements, inventory_reservations
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
// در inventory.go و inventory_db.go تعریف شده‌اند.
package inventory
