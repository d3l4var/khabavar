# khabavar PostgreSQL backup script (R-18 §۵ — nightly logical backup)
# Runs via cron: 0 2 * * * /opt/khabavar/bin/backup.sh
# Keeps 30 days of daily backups + 12 weeks of weekly backups.
set -euo pipefail

BACKUP_DIR="/var/backups/khabavar"
DB_NAME="khabavar"
RETENTION_DAILY=30
RETENTION_WEEKLY=84  # 12 weeks
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
DAILY_FILE="${BACKUP_DIR}/daily/khabavar_${TIMESTAMP}.sql.gz"
WEEKLY_FILE="${BACKUP_DIR}/weekly/khabavar_weekly_$(date +%Y%m%d).sql.gz"

mkdir -p "${BACKUP_DIR}/daily" "${BACKUP_DIR}/weekly"

echo "[$(date)] Starting backup of ${DB_NAME}..."

# Logical backup (pg_dump) with custom format, piped through gzip.
# Uses the khabavar system role (configured in pg_hba.conf).
pg_dump "${DB_NAME}" --no-owner --no-privileges --format=custom \
  | gzip > "${DAILY_FILE}"

echo "[$(date)] Daily backup: ${DAILY_FILE} ($(du -h "${DAILY_FILE}" | cut -f1))"

# Sunday: also copy to weekly.
if [ "$(date +%u)" = "7" ]; then
  cp "${DAILY_FILE}" "${WEEKLY_FILE}"
  echo "[$(date)] Weekly backup: ${WEEKLY_FILE}"
fi

# Prune old daily backups.
find "${BACKUP_DIR}/daily" -name "khabavar_*.sql.gz" -mtime +${RETENTION_DAILY} -delete
# Prune old weekly backups.
find "${BACKUP_DIR}/weekly" -name "khabavar_weekly_*.sql.gz" -mtime +${RETENTION_WEEKLY} -delete

echo "[$(date)] Backup complete. Pruned backups older than ${RETENTION_DAILY}d (daily) / ${RETENTION_WEEKLY}d (weekly)."
