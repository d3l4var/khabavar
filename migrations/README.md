# Migrations

ترتیب اجرا طبق DB v2 §۱۶:

۱. `000001_identity.up.sql`
۲. `000002_catalog.up.sql`
۳. `000003_supplier.up.sql`
۴. `000004_inventory.up.sql`
۵. `000005_pricing.up.sql`
۶. `000006_commerce.up.sql`
۷. `000007_fulfillment.up.sql`
۸. `000008_content.up.sql`
۹. `000009_audit.up.sql`
۱۰. `000010_db_roles.up.sql`
۱۱. `000011_oidc_refresh_tokens.up.sql`

هر Migration اجراشده تغییر نمی‌کند؛ seed جدا از migration اجرا می‌شود (DEP v1.1 §۶، CI §۴).
