// Package content implements the content/magazine module (SAD §۷.۲, DB v2 §۱۲,
// R-04 §۴, ADR-CONTENT-001): topics (category tree), articles with editorial
// workflow, article sources (the publish gate), product-article relations
// (§12.3 with a required relation_reason per MA-5), and language-variant
// translations (parent_article_id chain, SAD §۷.۲).
//
// مسئولیت: مقالات، دسته‌ها، منابع، ترجمه، بازبینی تحریریه، رابطه مقاله-محصول
// موجودیت‌ها: articles, topics, article_sources, article_topics, product_article_relations
// وابستگی مجاز (SAD §۴.۲): catalog (فقط id), identity (فقط id)
//
// قواعد وابستگی (الزامی، SAD §۴.۲):
//
//	۱. هیچ ماژولی جدول ماژول دیگر را مستقیم UPDATE/INSERT نمی‌کند.
//	۲. ارتباط بین‌ماژولی فقط از طریق API داخلی (متد در مرز ماژول) + رویداد Outbox.
//	۳. وابستگی دایره‌ای ممنوع.
//	۴. هر عملیات حساس: Idempotency Key + Audit (ADR-ARCH-008/010).
//
// نوع‌ها، Store و Service در content.go و content_db.go تعریف شده‌اند.
package content
