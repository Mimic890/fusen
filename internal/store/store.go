// Package store persists domain entities in SQLite.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // registers the "sqlite" driver

	"github.com/Mimic890/fusen/migrations"
)

// Store wraps the database handle.
type Store struct {
	db *sql.DB
}

// Open opens (creating if needed) the database at path and applies migrations.
func Open(ctx context.Context, path string) (*Store, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, permissionHint(dir, fmt.Errorf("create db dir: %w", err))
	}
	if err := checkWritable(dir, path); err != nil {
		return nil, err
	}

	// Pragmas are applied to every new connection by the driver.
	q := url.Values{}
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "synchronous(NORMAL)")
	q.Set("_txlock", "immediate")
	dsn := "file:" + path + "?" + q.Encode()

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}
	if err := migrate(ctx, db, migrations.FS); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

// ErrNotWritable means the database directory or file cannot be written.
var ErrNotWritable = errors.New("database location is not writable")

// checkWritable verifies that the bot can create files in dir (SQLite also
// needs it for the -wal and -shm files) and write the database file if it
// exists, so a permission problem is reported clearly instead of as
// SQLite's "unable to open database file".
func checkWritable(dir, path string) error {
	f, err := os.CreateTemp(dir, ".fusen-write-check-*")
	if err != nil {
		return permissionHint(dir, err)
	}
	name := f.Name()
	f.Close()
	if err := os.Remove(name); err != nil {
		return permissionHint(dir, err)
	}

	db, err := os.OpenFile(path, os.O_RDWR, 0)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		return permissionHint(dir, err)
	}
	return db.Close()
}

// permissionHint wraps permission errors with ErrNotWritable and a fix.
func permissionHint(dir string, err error) error {
	if !errors.Is(err, fs.ErrPermission) {
		return err
	}
	uid, gid := os.Getuid(), os.Getgid()
	return fmt.Errorf("%w: %s is not writable by uid %d, gid %d (%v); "+
		"if Docker created the directory as root, fix its owner on the host, e.g. "+
		"`sudo chown -R %d:%d ./data`, or set FUSEN_UID/FUSEN_GID to the owner of ./data",
		ErrNotWritable, dir, uid, gid, err, uid, gid)
}

// Close closes the database.
func (s *Store) Close() error {
	return s.db.Close()
}

// inTx runs fn in a transaction, committing on success.
func (s *Store) inTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
