// Package store persists domain values in SQLite with hand-written SQL.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"regexp"
	"sort"
	"strconv"

	_ "modernc.org/sqlite" // database/sql driver

	"workline/internal/domain"
	"workline/migrations"
)

// Sentinel errors returned (wrapped) by the store.
var (
	ErrNotFound   = errors.New("not found")
	ErrConflict   = errors.New("already exists")
	ErrReadOnly   = errors.New("entry is read-only")
	ErrNotFlagged = errors.New("activity is not flagged for scope creep")
	ErrNotEmpty   = errors.New("database is not empty")
)

// Store is a handle on one SQLite database.
type Store struct {
	db *sql.DB
}

// queryer is satisfied by *sql.DB and *sql.Tx so a query function can run in either.
type queryer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Open opens (creating if needed) the database at path and applies migrations.
func Open(path string) (*Store, error) {
	if path == "" {
		return nil, errors.New("store: empty database path")
	}
	dsn := "file:" + (&url.URL{Path: path}).EscapedPath() +
		"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	// One connection: single user, and it rules out SQLITE_BUSY between our own
	// goroutines. Code inside a transaction must use only that transaction.
	db.SetMaxOpenConns(1)
	if err := migrate(context.Background(), db, migrations.FS); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate %s: %w", path, err)
	}
	return &Store{db: db}, nil
}

// Close releases the database.
func (s *Store) Close() error { return s.db.Close() }

var migrationName = regexp.MustCompile(`^(\d{4})_[a-z0-9_]+\.sql$`)

type migration struct {
	version int
	name    string
}

// listMigrations returns the migration files in order, requiring 0001.. with no gaps.
func listMigrations(fsys fs.FS) ([]migration, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, err
	}
	var ms []migration
	for _, e := range entries {
		m := migrationName.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		v, _ := strconv.Atoi(m[1])
		ms = append(ms, migration{v, e.Name()})
	}
	sort.Slice(ms, func(i, j int) bool { return ms[i].version < ms[j].version })
	for i, m := range ms {
		if m.version != i+1 {
			return nil, fmt.Errorf("migration numbering: want %04d, found %s", i+1, m.name)
		}
	}
	return ms, nil
}

// migrate applies pending migrations, each in its own transaction, tracking
// progress in PRAGMA user_version.
func migrate(ctx context.Context, db *sql.DB, fsys fs.FS) error {
	ms, err := listMigrations(fsys)
	if err != nil {
		return err
	}
	var current int
	if err := db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&current); err != nil {
		return fmt.Errorf("read user_version: %w", err)
	}
	if current > len(ms) {
		return fmt.Errorf("database version %d is newer than this binary (%d)", current, len(ms))
	}
	for _, m := range ms[current:] {
		body, err := fs.ReadFile(fsys, m.name)
		if err != nil {
			return err
		}
		if err := applyMigration(ctx, db, m, string(body)); err != nil {
			return fmt.Errorf("apply %s: %w", m.name, err)
		}
	}
	return nil
}

func applyMigration(ctx context.Context, db *sql.DB, m migration, body string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, body); err != nil {
		return err
	}
	// PRAGMA does not accept bound parameters; m.version is an int we parsed.
	if _, err := tx.ExecContext(ctx, "PRAGMA user_version = "+strconv.Itoa(m.version)); err != nil {
		return err
	}
	return tx.Commit()
}

// inTx runs fn in a transaction, committing only if fn returns nil.
func (s *Store) inTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// mapErr turns SQLite constraint failures into the store's sentinels.
func mapErr(err error) error {
	var coded interface{ Code() int }
	if !errors.As(err, &coded) {
		return err
	}
	const (
		constraint = 19   // SQLITE_CONSTRAINT primary code
		unique     = 2067 // SQLITE_CONSTRAINT_UNIQUE
		primaryKey = 1555 // SQLITE_CONSTRAINT_PRIMARYKEY
	)
	switch code := coded.Code(); {
	case code == unique || code == primaryKey:
		return fmt.Errorf("%w: %v", ErrConflict, err)
	case code&0xff == constraint:
		return fmt.Errorf("%w: %v", domain.ErrInvalid, err)
	}
	return err
}

// affected returns ErrNotFound when res touched no rows.
func affected(res sql.Result, what string, id int64) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%s %d: %w", what, id, ErrNotFound)
	}
	return nil
}

// parseDate parses a stored date; the CHECK constraints make failure a bug.
func parseDate(s string) (domain.Date, error) {
	d, err := domain.ParseDate(s)
	if err != nil {
		return 0, fmt.Errorf("corrupt stored date: %w", err)
	}
	return d, nil
}

// parseOptDate parses a nullable stored date; NULL becomes the zero Date.
func parseOptDate(s sql.NullString) (domain.Date, error) {
	if !s.Valid {
		return 0, nil
	}
	return parseDate(s.String)
}

// nullDate converts a Date to a nullable SQL value; the zero Date is NULL.
func nullDate(d domain.Date) sql.NullString {
	return sql.NullString{String: d.String(), Valid: !d.IsZero()}
}
