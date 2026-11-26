// Package media implements the media module (SAD §۴.۲, DB v2 §۶ media_assets,
// R-04 §۸): media assets (images, videos, documents) and the Object Storage
// adapter abstraction.
//
// مسئولیت: دارایی‌های رسانه‌ای + Adapter Object Storage
// موجودیت‌ها: media_assets (در migration 000002_catalog تعریف شده)
// وابستگی مجاز (SAD §۴.۲): shared
//
// قواعد وابستگی (الزامی، SAD §۴.۲):
//
//	۱. هیچ ماژولی جدول ماژول دیگر را مستقیم UPDATE/INSERT نمی‌کند.
//	۲. ارتباط بین‌ماژولی فقط از طریق API داخلی (متد در مرز ماژول) + رویداد Outbox.
//	۳. وابستگی دایره‌ای ممنوع.
//	۴. هر عملیات حساس: Idempotency Key + Audit (ADR-ARCH-008/010).
//
// نقطه شروع پیاده‌سازی: IMPLEMENTATION_GUIDE.md فاز مربوطه. نوع‌ها، Store،
// StorageAdapter و Service در media.go و media_db.go تعریف شده‌اند.
package media
