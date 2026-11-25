#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

if [[ -z "${INFRAOS_POSTGRES_DSN:-}" ]]; then
  echo "INFRAOS_POSTGRES_DSN must be set before running PostgreSQL backup." >&2
  exit 1
fi
if ! command -v pg_dump >/dev/null 2>&1; then
  echo "pg_dump is not installed on this machine." >&2
  exit 1
fi

BACKUP_DIR="${INFRAOS_BACKUP_DIR:-./backups}"
mkdir -p "$BACKUP_DIR"
STAMP="$(date -u +%Y%m%d-%H%M%S)"
DEST="$BACKUP_DIR/infraos-postgres-${STAMP}.dump"

pg_dump --format=custom --no-owner --no-privileges --file="$DEST" "$INFRAOS_POSTGRES_DSN"
shasum -a 256 "$DEST" > "${DEST}.sha256"
echo "PostgreSQL custom-format backup created: $DEST"
cat "${DEST}.sha256"
