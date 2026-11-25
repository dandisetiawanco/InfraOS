#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

BACKUP_DIR="${INFRAOS_BACKUP_DIR:-./backups}"
LATEST="$(ls -1t "$BACKUP_DIR"/infraos-sqlite-*.db 2>/dev/null | head -n 1 || true)"

if [[ -z "$LATEST" ]]; then
  echo "No SQLite backup found in $BACKUP_DIR. Run ./scripts/backup-sqlite.sh first." >&2
  exit 1
fi

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

if [[ -f "${LATEST}.sha256" ]]; then
  shasum -a 256 -c "${LATEST}.sha256"
fi

RESTORED="$TMP_DIR/restored-test.db"
cp "$LATEST" "$RESTORED"
INTEGRITY="$(sqlite3 "$RESTORED" "PRAGMA integrity_check;")"
TABLES="$(sqlite3 "$RESTORED" "SELECT COUNT(*) FROM sqlite_master WHERE type='table';")"

if [[ "$INTEGRITY" != "ok" ]]; then
  echo "Integrity check failed: $INTEGRITY" >&2
  exit 1
fi

echo "Safe temporary restore verification passed:"
echo "  Source backup:  $LATEST"
echo "  Integrity:      $INTEGRITY"
echo "  Tables present: $TABLES"
echo "  Active database was untouched."
