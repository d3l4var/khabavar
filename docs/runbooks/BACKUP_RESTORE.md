# Runbook: Backup & Restore (R-18 §۵)

آخرین به‌روزرسانی: 2026-11-26

## هدف

این ران‌بوک راهبرد backup و restore دیتابیس PostgreSQL khabavar را توضیح می‌دهد — backup شبانه خودکار، نگه‌داری ۳۰ روز روزانه + ۱۲ هفته هفتگی، و فرآیند restore.

## راهبرد Backup

### فرکانس و نگه‌داری

| نوع | فرکانس | نگه‌داری | مسیر |
|------|--------|---------|------|
| روزانه | هر شب ساعت ۰۲:۰۰ | ۳۰ روز | `/var/backups/khabavar/daily/` |
| هفتگی | یکشنبه ساعت ۰۲:۰۰ | ۱۲ هفته | `/var/backups/khabavar/weekly/` |

### نوع backup

`pg_dump --format=custom` — backup منطقی با فرمت فشرده. مزایا:
- مستقل از نسخه PostgreSQL (قابل restore روی نسخه‌های دیگر)
- فشرده (gzip)
- قابل بازگردانی انتخابی (جدول/اسکما به‌جدول)

### راه‌اندازی cron

```bash
# crontab -e (کاربر root)
0 2 * * * /opt/khabavar/deploy/scripts/backup.sh >> /var/log/khabavar-backup.log 2>&1
```

### بررسی خودکار backup

هر هفته یک اسکریپت بررسی می‌کند که backup آخرین ۲۴ ساعت موجود است:

```bash
# weekly check
find /var/backups/khabavar/daily -name "*.sql.gz" -mtime -1 | grep -q . \
  || echo "ALERT: no backup in last 24h" | mail -s "khabavar backup alert" ops@khabavar.ir
```

## Restore

### Restore کامل (پس از فاجعه)

```bash
# ۱. آخرین backup را پیدا کن
ls -lt /var/backups/khabavar/daily/ | head

# ۲. restore را اجرا کن (تأیید می‌خواهد)
/opt/khabavar/deploy/scripts/restore.sh /var/backups/khabavar/daily/khabavar_YYYYMMDD_HHMMSS.sql.gz
```

اسکریپت `restore.sh`:
- سه unit را متوقف می‌کند
- دیتابیس را DROP و CREATE می‌کند
- backup را restore می‌کند
- migrations را تا آخرین نسخه اعمال می‌کند
- unit‌ها را شروع می‌کند

### Restore به محیط staging (آزمایش)

```bash
# backup تولید را روی staging تست کن قبل از اعتماد کامل
createdb khabavar_staging
gunzip -c /var/backups/khabavar/daily/khabavar_*.sql.gz | pg_restore --dbname=khabavar_staging --no-owner
# بررسی یکپارچگی داده
psql -d khabavar_staging -c "SELECT count(*) FROM products WHERE status='PUBLISHED';"
```

### Point-in-time Recovery (اختیاری)

اگر PostgreSQL WAL archiving فعال است (توصیه می‌شود برای production):

```conf
# postgresql.conf
archive_mode = on
archive_command = 'test ! -f /var/backups/wal/%f && cp %p /var/backups/wal/%f'
wal_level = replica
```

این اجازه می‌دهد restore تا یک نقطه زمانی خاص انجام شود (مثلاً قبل از یک DELETE اشتباه).

## آزمایش بازیابی (Restore Test)

هر ماه یک restore تست روی staging انجام دهید (R-18 §۵ معیار پذیرش):

1. آخرین backup روزانه را روی staging restore کن
2. بررسی کن که تعداد محصولات/مقالات/سفارش‌ها با تولید مطابقت دارد
3. یک سفارش تست ثبت کن و بررسی کن که چرخه کامل کار می‌کند
4. نتیجه را در `/var/log/khabavar-restore-test.log` ثبت کن

## چک‌لیست

- [ ] cron backup فعال و در حال اجرا
- [ ] لاگ backup هفتگی بررسی می‌شود
- [ ] فضای دیسک `/var/backups` ≥ ۵× اندازه backup روزانه
- [ ] restore تست ماهانه انجام و ثبت می‌شود
- [ ] WAL archiving فعال (برای point-in-time)
- [ ] backup‌ها به ذخیره‌ساز خارجی (S3/rsync) هم کپی می‌شوند
