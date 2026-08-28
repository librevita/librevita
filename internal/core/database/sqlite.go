// Package database provides the LibreVita connection factory.
//
// modernc.org/sqlite is pure Go, so this package remains statically
// cross-compilable for riscv64, mips, loong64, and other targets.
package database

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/cockroachdb/errors"
	_ "modernc.org/sqlite" // registers the "sqlite" driver (pure Go, no CGO)
)

// sqliteDriver is the driver name registered by modernc.org/sqlite.
const sqliteDriver = "sqlite"

// openSQLite opens SQLite with the required operational pragmas:
//
//   - journal_mode(WAL): concurrent reads and writes.
//   - busy_timeout: wait instead of returning SQLITE_BUSY under contention.
//   - foreign_keys(on): enforce referential integrity.
//   - synchronous(NORMAL): balanced durability for WAL-backed edge storage.
func openSQLite(path string) (*sql.DB, error) {
	if err := ensureParentDir(path); err != nil {
		return nil, errors.Wrapf(err, "sqlite: failed to create parent directory for %q", path)
	}

	q := url.Values{}
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "foreign_keys(on)")
	q.Add("_pragma", "synchronous(NORMAL)")

	dsn := fmt.Sprintf("file:%s?%s", path, q.Encode())

	db, err := sql.Open(sqliteDriver, dsn)
	if err != nil {
		return nil, errors.Wrapf(err, "sqlite: failed to open %q", path)
	}

	// SQLite has a single writer. A one-connection pool serializes writes
	// and avoids multiple connections competing for the WAL lock.
	db.SetMaxOpenConns(1)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, errors.Wrapf(err, "sqlite: ping failed for %q", path)
	}
	return db, nil
}

func ensureParentDir(path string) error {
	dir := filepath.Dir(path)
	if dir == "." || dir == "" {
		return nil
	}
	return os.MkdirAll(dir, 0o750)
}
