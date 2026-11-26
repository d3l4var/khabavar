# Runbook: Deployment (R-18 §۳)

آخرین به‌روزرسانی: 2026-11-26

## هدف

این ران‌بوک فرآیند استقرار khabavar را روی سرور تولید توضیح می‌دهد — از build تا راه‌اندازی سه unit (identity / api / worker) و نهایی‌سازی nginx.

## پیش‌نیازها

- سرور Ubuntu 22.04+ با دسترسی root/sudo
- PostgreSQL 15+ نصب و در حال اجرا
- nginx نصب شده
- Go 1.22+ (یا بایناری از CI)
- گواهی TLS (Let's Encrypt)

## مراحل استقرار

### ۱. Build باینری

```bash
# روی ماشین build (یا CI):
cd /path/to/khabavar
go build -o bin/khabavar ./cmd/khabavar
# cross-compile برای لینوکس اگر روی macOS build می‌گیرید:
# GOOS=linux GOARCH=amd64 go build -o bin/khabavar ./cmd/khabavar
```

### ۲. انتقال به سرور

```bash
scp bin/khabavar deploy/ root@server:/tmp/khabavar-deploy/
```

### ۳. نصب روی سرور

```bash
ssh root@server

# کاربر و گروه
useradd --system --no-create-home --shell /usr/sbin/nologin khabavar

# مسیرها
mkdir -p /opt/khabavar/bin /var/lib/khabavar/media /var/www/khabavar /var/backups/khabavar /etc/khabavar
chown -R khabavar:khabavar /opt/khabavar /var/lib/khabavar /var/backups/khabavar

# باینری
cp /tmp/khabavar-deploy/khabavar /opt/khabavar/bin/
chmod +x /opt/khabavar/bin/khabavar

# systemd units
cp /tmp/khabavar-deploy/systemd/*.service /etc/systemd/system/
systemctl daemon-reload

# env file (اسرار — هرگز در git)
cat > /etc/khabavar/env <<EOF
KH_TOKEN_SIGNING_KEY=<HMAC key برای OTP/OIDC>
KH_OIDC_ISSUER=https://id.khabavar.ir
KH_OTP_TTL_SECONDS=120
KH_OTP_MAX_PER_HOUR=5
KH_ACCESS_TOKEN_TTL=900
KH_REFRESH_TOKEN_TTL=2592000
KH_DB_MAX_OPEN_CONNS=25
EOF
chmod 600 /etc/khabavar/env
chown khabavar:khabavar /etc/khabavar/env
```

### ۴. دیتابیس

```bash
sudo -u postgres createuser khabavar
sudo -u postgres createdb khabavar --owner=khabavar
sudo -u postgres psql -c "ALTER USER khabavar PASSWORD '...';"

# migrations (بالا به پایین)
for f in migrations/0000*_*.up.sql; do
  sudo -u khabavar psql -d khabavar -f "$f"
done

# seed (اختیاری، داده اولیه)
sudo -u khabavar psql -d khabavar -f scripts/seed.sql
```

### ۵. nginx + TLS

```bash
cp /tmp/khabavar-deploy/nginx/khabavar.conf /etc/nginx/sites-available/
ln -sf /etc/nginx/sites-available/khabavar.conf /etc/nginx/sites-enabled/
nginx -t && systemctl reload nginx

# TLS (Let's Encrypt)
certbot --nginx -d khabavar.ir -d shop.khabavar.ir -d magazine.khabavar.ir -d admin.khabavar.ir -d id.khabavar.ir
```

### ۶. شروع سرویس‌ها

```bash
systemctl enable --now khabavar-identity khabavar-api khabavar-worker
systemctl status khabavar-identity khabavar-api khabavar-worker
```

### ۷. بررسی سلامت

```bash
curl -sf https://khabavar.ir/healthz | jq .
curl -sf https://id.khabavar.ir/healthz | jq .
curl -sf https://shop.khabavar.ir/api/v1/categories | jq '.data | length'
```

هر سه باید `200 OK` برگردانند. اگر نه، لاگ‌ها را بررسی کنید:

```bash
journalctl -u khabavar-api -u khabavar-identity -u khabavar-worker --since "5 min ago"
```

## استقرار به‌روزرسانی (Rolling)

```bash
# ۱. build جدید را کپی کن
scp bin/khabavar root@server:/opt/khabavar/bin/khabavar.new
ssh root@server

# ۲. unit‌ها را یکی‌یکی restart کن (api اول، بعد worker، بعد identity)
mv /opt/khabavar/bin/khabavar.new /opt/khabavar/bin/khabavar
chmod +x /opt/khabavar/bin/khabavar
systemctl restart khabavar-api && sleep 2
systemctl restart khabavar-worker && sleep 2
systemctl restart khabavar-identity
```

## Rollback

اگر به‌روزرسانی مشکل ایجاد کرد:

```bash
# باینری قبلی را بازگردان (نگه‌داشته در /opt/khabavar/bin/khabavar.prev)
mv /opt/khabavar/bin/khabavar /opt/khabavar/bin/khabavar.bad
mv /opt/khabavar/bin/khabavar.prev /opt/khabavar/bin/khabavar
systemctl restart khabavar-api khabavar-worker khabavar-identity
```

## نکات

- هرگز `KH_TOKEN_SIGNING_KEY` را در git commit نکنید — فقط در `/etc/khabavar/env`.
- قبل از هر استقرار، backup بگیرید (`deploy/scripts/backup.sh`).
- migrations فقط به جلو اعمال می‌شوند (never down در production مگر با تأیید مالک پروژه).
