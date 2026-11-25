package database

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dandisetiawanco/InfraOS/database/migrations"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

type DB struct {
	SQL    *sql.DB
	Driver string
	Path   string
}

type Querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func Open(ctx context.Context, driver, sqlitePath, postgresDSN string) (*DB, error) {
	switch driver {
	case "sqlite":
		if sqlitePath != ":memory:" && !strings.HasPrefix(sqlitePath, "file:") {
			if err := os.MkdirAll(filepath.Dir(sqlitePath), 0o750); err != nil {
				return nil, fmt.Errorf("create sqlite directory: %w", err)
			}
		}
		sqlDB, err := sql.Open("sqlite", sqlitePath)
		if err != nil {
			return nil, fmt.Errorf("open sqlite: %w", err)
		}
		// Serialize writes cleanly in lightweight mode to avoid SQLITE_BUSY under concurrent goroutines.
		sqlDB.SetMaxOpenConns(1)
		sqlDB.SetMaxIdleConns(1)
		pragmas := []string{
			"PRAGMA foreign_keys = ON;",
			"PRAGMA journal_mode = WAL;",
			"PRAGMA busy_timeout = 5000;",
			"PRAGMA synchronous = NORMAL;",
		}
		for _, p := range pragmas {
			if _, err := sqlDB.ExecContext(ctx, p); err != nil {
				_ = sqlDB.Close()
				return nil, fmt.Errorf("apply sqlite pragma %q: %w", p, err)
			}
		}
		return &DB{SQL: sqlDB, Driver: "sqlite", Path: sqlitePath}, nil

	case "postgres":
		sqlDB, err := sql.Open("pgx", postgresDSN)
		if err != nil {
			return nil, fmt.Errorf("open postgres: %w", err)
		}
		sqlDB.SetMaxOpenConns(20)
		sqlDB.SetMaxIdleConns(5)
		sqlDB.SetConnMaxLifetime(30 * time.Minute)
		if err := sqlDB.PingContext(ctx); err != nil {
			_ = sqlDB.Close()
			return nil, fmt.Errorf("ping postgres: %w", err)
		}
		return &DB{SQL: sqlDB, Driver: "postgres"}, nil

	default:
		return nil, fmt.Errorf("unsupported database driver: %s", driver)
	}
}

func (d *DB) Close() error {
	if d == nil || d.SQL == nil {
		return nil
	}
	return d.SQL.Close()
}

func (d *DB) Ping(ctx context.Context) error {
	return d.SQL.PingContext(ctx)
}

// Rebind converts positional '?' placeholders to PostgreSQL '$1, $2...' placeholders.
func (d *DB) Rebind(query string) string {
	if d.Driver != "postgres" {
		return query
	}
	var b strings.Builder
	b.Grow(len(query) + 16)
	argIdx := 1
	inSingleQuote := false
	for i := 0; i < len(query); i++ {
		ch := query[i]
		if ch == '\'' {
			inSingleQuote = !inSingleQuote
			b.WriteByte(ch)
			continue
		}
		if ch == '?' && !inSingleQuote {
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(argIdx))
			argIdx++
			continue
		}
		b.WriteByte(ch)
	}
	return b.String()
}

func (d *DB) Exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return d.SQL.ExecContext(ctx, d.Rebind(query), args...)
}

func (d *DB) Query(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return d.SQL.QueryContext(ctx, d.Rebind(query), args...)
}

func (d *DB) QueryRow(ctx context.Context, query string, args ...any) *sql.Row {
	return d.SQL.QueryRowContext(ctx, d.Rebind(query), args...)
}

func (d *DB) WithTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := d.SQL.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if r := recover(); r != nil {
			_ = tx.Rollback()
			panic(r)
		}
	}()
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (d *DB) Migrate(ctx context.Context) error {
	createMeta := `CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at TEXT NOT NULL
	);`
	if _, err := d.SQL.ExecContext(ctx, createMeta); err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}

	dir := d.Driver
	entries, err := fs.ReadDir(migrations.FS, dir)
	if err != nil {
		return fmt.Errorf("read embedded migrations for %s: %w", dir, err)
	}

	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			files = append(files, entry.Name())
		}
	}
	sort.Strings(files)

	for _, name := range files {
		var existing string
		err := d.QueryRow(ctx, "SELECT version FROM schema_migrations WHERE version = ?", name).Scan(&existing)
		if err == nil {
			continue
		}
		if err != sql.ErrNoRows {
			return fmt.Errorf("check migration %s: %w", name, err)
		}

		raw, err := fs.ReadFile(migrations.FS, filepath.Join(dir, name))
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}

		err = d.WithTx(ctx, func(tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, string(raw)); err != nil {
				return fmt.Errorf("execute migration %s: %w", name, err)
			}
			_, err := tx.ExecContext(ctx, d.Rebind("INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)"), name, time.Now().UTC().Format(time.RFC3339))
			return err
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// BackupSQLite creates a consistent point-in-time SQLite snapshot via VACUUM INTO.
func (d *DB) BackupSQLite(ctx context.Context, destPath string) (int64, string, error) {
	if d.Driver != "sqlite" {
		return 0, "", fmt.Errorf("sqlite backup requested for %s driver", d.Driver)
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0o750); err != nil {
		return 0, "", err
	}
	absPath, err := filepath.Abs(destPath)
	if err != nil {
		return 0, "", err
	}
	_ = os.Remove(absPath)

	escaped := strings.ReplaceAll(absPath, "'", "''")
	if _, err := d.SQL.ExecContext(ctx, fmt.Sprintf("VACUUM INTO '%s';", escaped)); err != nil {
		return 0, "", fmt.Errorf("vacuum into backup: %w", err)
	}

	f, err := os.Open(absPath)
	if err != nil {
		return 0, "", err
	}
	defer f.Close()

	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return 0, "", err
	}
	return n, hex.EncodeToString(h.Sum(nil)), nil
}

// PruneRetention deletes expired time-series metrics, service checks, and log entries.
func (d *DB) PruneRetention(ctx context.Context, metricsDays, logsDays, checksDays int) (int64, error) {
	var total int64
	now := time.Now().UTC()
	tables := []struct {
		query  string
		cutoff time.Time
	}{
		{"DELETE FROM host_metrics WHERE recorded_at < ?", now.AddDate(0, 0, -metricsDays)},
		{"DELETE FROM log_entries WHERE timestamp < ?", now.AddDate(0, 0, -logsDays)},
		{"DELETE FROM service_checks WHERE checked_at < ?", now.AddDate(0, 0, -checksDays)},
		{"DELETE FROM sessions WHERE expires_at < ?", now.Add(-24 * time.Hour)},
	}
	for _, item := range tables {
		res, err := d.Exec(ctx, item.query, item.cutoff.Format(time.RFC3339))
		if err != nil {
			return total, err
		}
		n, _ := res.RowsAffected()
		total += n
	}
	return total, nil
}

func BoolToDB(driver string, v bool) any {
	if driver == "postgres" {
		return v
	}
	if v {
		return 1
	}
	return 0
}

func NowUTC() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}
