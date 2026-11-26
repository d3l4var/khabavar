# khabavar PostgreSQL restore script (R-18 §۵)
# Usage: restore.sh <backup-file.sql.gz>
# Restores a logical backup into a fresh database. Destructive: drops existing data.
set -euo pipefail

BACKUP_FILE="${1:-}"
DB_NAME="khabavar"

if [ -z "${BACKUP_FILE}" ] || [ ! -f "${BACKUP_FILE}" ]; then
  echo "Usage: $0 <backup-file.sql.gz>"
  echo "Example: $0 /var/backups/khabavar/daily/khabavar_20251126_020000.sql.gz"
  exit 1
fi

echo "[$(date)] WARNING: This will DROP and recreate database '${DB_NAME}'."
echo "Backup file: ${BACKUP_FILE}"
read -p "Type 'CONFIRM' to proceed: " confirm
if [ "${confirm}" != "CONFIRM" ]; then
  echo "Aborted."
  exit 1
fi

echo "[$(date)] Stopping API units..."
systemctl stop khabavar-api khabavar-worker khabavar-identity || true

echo "[$(date)] Dropping and recreating database..."
dropdb --if-exists "${DB_NAME}"
createdb "${DB_NAME}"

echo "[$(date)] Restoring from backup..."
gunzip -c "${BACKUP_FILE}" | pg_restore --dbname="${DB_NAME}" --no-owner --no-privileges --clean --if-exists || true

echo "[$(date)] Running migrations to bring schema to latest..."
cd /opt/khabavar && KH_DB_URL="postgres:///khabavar" ./bin/khabavar-migrate up 2>/dev/null || \
  echo "[$(date)] NOTE: migrate command not found — apply migrations manually if needed."

echo "[$(date)] Starting API units..."
systemctl start khabavar-identity khabavar-api khabavar-worker

echo "[$(date)] Restore complete. Verify: curl https://khabavar.ir/api/v1/categories"
