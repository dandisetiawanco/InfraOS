# Backup and Recovery Procedures

## Consistent Backup Methods

InfraOS never copies an open, actively written database file directly.

### SQLite (Lightweight Profile)
- Uses SQLite `VACUUM INTO '<destination>'` to generate a transactionally consistent snapshot while readers continue serving traffic.
- Runs `PRAGMA integrity_check` and generates a SHA-256 checksum file (`*.sha256`).
- Accessible both via CLI (`make backup-sqlite` -> `scripts/backup-sqlite.sh`) and via the authenticated workspace **Settings -> Create SQLite backup** action (`POST /api/v1/backups`).

### PostgreSQL (Infrastructure & Production Profiles)
- Uses `pg_dump --format=custom --no-owner --no-privileges` (`scripts/backup-postgres.sh`) with SHA-256 verification.

## Safe Restore Verification (Non-Destructive)

Verify SQLite backup recoverability without touching the live database:

```bash
make test-restore-sqlite
```

`scripts/test-restore-sqlite.sh` validates the SHA-256 digest, copies the latest snapshot into an isolated temporary directory, confirms `PRAGMA integrity_check` returns `ok`, counts restored tables, and cleans up the temp directory.

## Destructive Production Restore Protocol

Restores are never executed automatically over active databases. To restore manually:
1. Stop `infraos-api` and `infraos-worker` (`make stop` or `systemctl stop infraos-api infraos-worker`).
2. Verify the backup checksum (`shasum -a 256 -c <backup>.sha256`).
3. Archive the existing database file or PostgreSQL database before replacement.
4. For SQLite, replace `data/infraos.db` (removing stale `-wal` and `-shm` files). For PostgreSQL, restore with `pg_restore --clean --if-exists --dbname="$INFRAOS_POSTGRES_DSN" <backup>.dump`.
5. Restart services and verify `GET /readyz`.

## Recovery Targets (Not Measured Guarantees)

- Target RPO (Recovery Point Objective): 24 hours with daily scheduled snapshots (or tighter if continuous WAL archiving is added in production).
- Target RTO (Recovery Time Objective): Under 30 minutes for single-node restoration.
