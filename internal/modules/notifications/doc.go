// Package notifications implements the notifications module (SAD §۴.۲, ADR-ARCH-011):
// SMS and email delivery adapters, plus a dispatcher that consumes domain events
// from the outbox and routes them to the right channel.
//
// مسئولیت: اعلان‌ها (SMS + Email adapters + dispatcher)
// وابستگی مجاز (SAD §۴.۲): shared
//
// قواعد وابستگی (الزامی، SAD §۴.۲):
//
//	۱. هیچ ماژولی جدول ماژول دیگر را مستقیم UPDATE/INSERT نمی‌کند.
//	۲. ارتباط بین‌ماژولی فقط از طریق API داخلی (متد در مرز ماژول) + رویداد Outbox.
//	۳. وابستگی دایره‌ای ممنوع.
//	۴. هر عملیات حساس: Idempotency Key + Audit (ADR-ARCH-008/010).
//
// نوع‌ها، SmsAdapter/EmailAdapter و Dispatcher در notifications.go تعریف شده‌اند.
package notifications
