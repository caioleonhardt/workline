package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"workline/internal/domain"
	"workline/migrations"
)

var ctx = context.Background()

func date(t *testing.T, s string) domain.Date {
	t.Helper()
	d, err := domain.ParseDate(s)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// newTestStore opens a fresh migrated database in a temp dir.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// mustActivity creates a project and an activity in it.
func mustActivity(t *testing.T, s *Store, project string, start, due string) domain.Activity {
	t.Helper()
	a, err := s.CreateActivity(ctx, NewActivity{NewProjectName: project, Name: "act", Start: date(t, start), Due: date(t, due), Subtasks: []string{"first"}}, fixed(date(t, "2026-01-01")))
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// fixed returns a clock standing at 09:30 on day d.
func fixed(d domain.Date) domain.Clock { return domain.FixedClock{D: d, Hour: 9, Minute: 30} }

func userVersion(t *testing.T, s *Store) int {
	t.Helper()
	var v int
	if err := s.db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestOpenAppliesMigrationsAndPragmas(t *testing.T) {
	path := filepath.Join(t.TempDir(), "w.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if v := userVersion(t, s); v != 3 {
		t.Errorf("user_version = %d, want 3", v)
	}
	var fk int
	var mode string
	s.db.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&fk)
	s.db.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&mode)
	if fk != 1 || mode != "wal" {
		t.Errorf("foreign_keys=%d journal_mode=%s", fk, mode)
	}
	if _, err := s.CreateProject(ctx, "keep"); err != nil {
		t.Fatal(err)
	}
	s.Close()

	s2, err := Open(path) // idempotent re-open keeps data
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	ps, err := s2.ListProjects(ctx, true)
	if err != nil || len(ps) != 1 || userVersion(t, s2) != 3 {
		t.Errorf("reopen: %v %v", ps, err)
	}
}

func TestOpenRejectsEmptyPath(t *testing.T) {
	if _, err := Open(""); err == nil {
		t.Error("expected error")
	}
}

func TestMigrateFailureRollsBack(t *testing.T) {
	s := newTestStore(t)
	fsys := fstest.MapFS{
		"0001_init.sql": {Data: []byte(`;`)},
		"0002_more.sql": {Data: []byte(`;`)},
		"0003_more.sql": {Data: []byte(`;`)},
		"0004_bad.sql":  {Data: []byte(`CREATE TABLE y (a INTEGER); SELECT * FROM nonexistent_table; CREATE TABLE z (b);`)},
	}
	// Versions 1 to 3 are already applied, so only 0004 runs.
	err := migrate(ctx, s.db, fsys)
	if err == nil {
		t.Fatal("expected failure")
	}
	if v := userVersion(t, s); v != 3 {
		t.Errorf("user_version = %d, want 3 (unchanged)", v)
	}
	var n int
	s.db.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE name IN ('y','z')`).Scan(&n)
	if n != 0 {
		t.Errorf("failed migration left %d tables behind", n)
	}
}

func TestMigrateAppliesNewFile(t *testing.T) {
	s := newTestStore(t)
	fsys := fstest.MapFS{
		"0001_init.sql": {Data: []byte(`-- already applied`)},
		"0002_more.sql": {Data: []byte(`-- already applied`)},
		"0003_more.sql": {Data: []byte(`-- already applied`)},
		"0004_more.sql": {Data: []byte(`CREATE TABLE extra (a INTEGER);`)},
	}
	if err := migrate(ctx, s.db, fsys); err != nil {
		t.Fatal(err)
	}
	if v := userVersion(t, s); v != 4 {
		t.Errorf("user_version = %d", v)
	}
}

func TestMigrateRefusals(t *testing.T) {
	s := newTestStore(t)
	if err := migrate(ctx, s.db, fstest.MapFS{}); err == nil || !strings.Contains(err.Error(), "newer") {
		t.Errorf("newer DB: %v", err)
	}
	gap := fstest.MapFS{"0001_a.sql": {Data: []byte(`;`)}, "0005_c.sql": {Data: []byte(`;`)}}
	if err := migrate(ctx, s.db, gap); err == nil || !strings.Contains(err.Error(), "numbering") {
		t.Errorf("gap: %v", err)
	}
	ignored := fstest.MapFS{"0001_a.sql": {Data: []byte(`;`)}, "0002_b.sql": {Data: []byte(`;`)}, "0003_c.sql": {Data: []byte(`;`)}, "README.md": {Data: []byte(`x`)}}
	if err := migrate(ctx, s.db, ignored); err != nil {
		t.Errorf("non-migration file should be ignored: %v", err)
	}
}

func TestMigration0002BackfillsDoneOn(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "old.db")+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	first, err := migrations.FS.ReadFile("0001_init.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err := migrate(ctx, db, fstest.MapFS{"0001_init.sql": {Data: first}}); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO projects (id, name) VALUES (1, 'P')`,
		`INSERT INTO activities (project_id, name, status, start_date, due_date) VALUES (1, 'a', 'done', '2026-01-01', '2026-02-01')`,
		`INSERT INTO activities (project_id, name, status, start_date, due_date) VALUES (1, 'b', 'active', '2026-01-01', '2026-02-01')`,
	} {
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	if err := migrate(ctx, db, migrations.FS); err != nil {
		t.Fatal(err)
	}
	var done, active sql.NullString
	db.QueryRowContext(ctx, `SELECT done_on FROM activities WHERE name = 'a'`).Scan(&done)
	db.QueryRowContext(ctx, `SELECT done_on FROM activities WHERE name = 'b'`).Scan(&active)
	if done.String != "2026-02-01" || active.Valid {
		t.Errorf("done_on: done=%v active=%v", done, active)
	}
}
