#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

DB_PATH="${INFRAOS_SQLITE_PATH:-./data/infraos.db}"
BACKUP_DIR="${INFRAOS_BACKUP_DIR:-./backups}"
RETENTION_COUNT="${INFRAOS_BACKUP_RETENTION:-7}"

if [[ ! -f "$DB_PATH" ]]; then
  echo "SQLite database not found at $DB_PATH" >&2
  exit 1
fi

mkdir -p "$BACKUP_DIR"
STAMP="$(date -u +%Y%m%d-%H%M%S)"
DEST="$BACKUP_DIR/infraos-sqlite-${STAMP}.db"

# Use SQLite VACUUM INTO for a transactionally consistent snapshot without locking readers.
sqlite3 "$DB_PATH" "VACUUM INTO '${DEST}';"
sqlite3 "$DEST" "PRAGMA integrity_check;" | grep -qx "ok"
shasum -a 256 "$DEST" > "${DEST}.sha256"

# Enforce retention count
ls -1t "$BACKUP_DIR"/infraos-sqlite-*.db 2>/dev/null | tail -n +$((RETENTION_COUNT + 1)) | while read -r old; do
  rm -f "$old" "${old}.sha256"
done

echo "Verified SQLite backup created: $DEST"
cat "${DEST}.sha256"
