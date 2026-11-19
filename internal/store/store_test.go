package store

import (
	"context"
	"database/sql"
	"errors"
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
	a, err := s.CreateActivity(ctx, NewActivity{NewProjectName: project, Name: "act", Start: date(t, start), Due: date(t, due), Tasks: []string{"first"}}, fixed(date(t, "2026-01-01")))
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
	if v := userVersion(t, s); v != 5 {
		t.Errorf("user_version = %d, want 5", v)
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
	if err != nil || len(ps) != 1 || userVersion(t, s2) != 5 {
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
		"0004_more.sql": {Data: []byte(`;`)},
		"0005_more.sql": {Data: []byte(`;`)},
		"0006_bad.sql":  {Data: []byte(`CREATE TABLE y (a INTEGER); SELECT * FROM nonexistent_table; CREATE TABLE z (b);`)},
	}
	// Versions 1 to 5 are already applied, so only 0006 runs.
	err := migrate(ctx, s.db, fsys)
	if err == nil {
		t.Fatal("expected failure")
	}
	if v := userVersion(t, s); v != 5 {
		t.Errorf("user_version = %d, want 5 (unchanged)", v)
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
		"0004_more.sql": {Data: []byte(`-- already applied`)},
		"0005_more.sql": {Data: []byte(`-- already applied`)},
		"0006_more.sql": {Data: []byte(`CREATE TABLE extra (a INTEGER);`)},
	}
	if err := migrate(ctx, s.db, fsys); err != nil {
		t.Fatal(err)
	}
	if v := userVersion(t, s); v != 6 {
		t.Errorf("user_version = %d", v)
	}
}

func TestMigrateRefusals(t *testing.T) {
	s := newTestStore(t)
	if err := migrate(ctx, s.db, fstest.MapFS{}); err == nil || !strings.Contains(err.Error(), "newer") {
		t.Errorf("newer DB: %v", err)
	}
	gap := fstest.MapFS{"0001_a.sql": {Data: []byte(`;`)}, "0006_c.sql": {Data: []byte(`;`)}}
	if err := migrate(ctx, s.db, gap); err == nil || !strings.Contains(err.Error(), "numbering") {
		t.Errorf("gap: %v", err)
	}
	ignored := fstest.MapFS{"0001_a.sql": {Data: []byte(`;`)}, "0002_b.sql": {Data: []byte(`;`)}, "0003_c.sql": {Data: []byte(`;`)}, "0004_d.sql": {Data: []byte(`;`)}, "0005_e.sql": {Data: []byte(`;`)}, "README.md": {Data: []byte(`x`)}}
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

// oldDB creates a database at schema 1 holding one project.
func oldDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	first, _ := migrations.FS.ReadFile("0001_init.sql")
	if err := migrate(ctx, db, fstest.MapFS{"0001_init.sql": {Data: first}}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO projects (id, name) VALUES (1, 'P')`); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestOpenRefusesAnOutdatedDatabase(t *testing.T) {
	path := oldDB(t)
	if _, err := Open(path); !errors.Is(err, ErrMigrationNeeded) {
		t.Fatalf("Open = %v, want ErrMigrationNeeded", err)
	}
	if have, _, err := SchemaVersions(ctx, path); err != nil || have != 1 {
		t.Errorf("Open changed the schema: %d %v", have, err)
	}
}

func TestMigrateUpgradesThenIsANoOp(t *testing.T) {
	path := oldDB(t)
	m, err := Migrate(ctx, path)
	if err != nil || m.From != 1 || m.To != 5 || m.Before["projects"] != 1 || m.After["projects"] != 1 {
		t.Fatalf("Migrate = %+v, %v", m, err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ps, err := s.ListProjects(ctx, true)
	s.Close()
	if err != nil || len(ps) != 1 {
		t.Errorf("data after migrating: %v %v", ps, err)
	}
	if m, err := Migrate(ctx, path); err != nil || m.From != m.To || m.Before != nil {
		t.Errorf("second Migrate = %+v, %v", m, err)
	}
	if _, err := Migrate(ctx, filepath.Join(t.TempDir(), "missing.db")); err == nil {
		t.Error("a missing database was accepted")
	}
}

func TestMigration0005MovesTasksIntoTheJournal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v4.db")
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	upTo4 := fstest.MapFS{}
	for _, name := range []string{"0001_init.sql", "0002_priority_done_on.sql", "0003_journal_logged_at.sql", "0004_subtask_closed_at.sql"} {
		body, err := migrations.FS.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		upTo4[name] = &fstest.MapFile{Data: body}
	}
	if err := migrate(ctx, db, upTo4); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO projects (id, name) VALUES (1, 'P')`,
		`INSERT INTO activities (id, project_id, name, status, start_date, due_date) VALUES (1, 1, 'a', 'active', '2026-01-01', '2026-12-31')`,
		`INSERT INTO subtasks (id, activity_id, text, created_on, closed_on, closed_at) VALUES
		   (1, 1, 'kickoff', '2026-01-01', '2026-02-01', '2026-02-01 10:15'),
		   (2, 1, 'kickoff 2', '2026-01-01', NULL, NULL),
		   (3, 1, 'deleted', '2026-03-01', NULL, NULL),
		   (4, 1, 'after ack', '2026-11-01', '2026-11-05', NULL),
		   (5, 1, 'after ack 2', '2026-11-10', NULL, NULL)`,
		`DELETE FROM subtasks WHERE id = 3`,
		`INSERT INTO journal_entries (id, activity_id, date, kind, text, logged_at) VALUES
		   (1, 1, '2026-01-01', 'update', 'Activity created', '2026-01-01 09:00'),
		   (2, 1, '2026-06-01', 'scope_ack', 'Scope change acknowledged', NULL)`,
		`INSERT INTO scope_acks (activity_id, date, baseline_count, last_subtask_id) VALUES (1, '2026-06-01', 2, 3)`,
	} {
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()

	d := func(s string) domain.Date { return date(t, s) }
	today := d("2026-11-18")
	act := domain.Activity{ID: 1, ProjectID: 1, Name: "a", Status: domain.StatusActive, Start: d("2026-01-01"), Due: d("2026-12-31")}
	before := domain.ScopeCreep(act, []domain.Task{
		{ID: 1, CreatedOn: d("2026-01-01"), ClosedOn: d("2026-02-01")}, {ID: 2, CreatedOn: d("2026-01-01")},
		{ID: 4, CreatedOn: d("2026-11-01"), ClosedOn: d("2026-11-05")}, {ID: 5, CreatedOn: d("2026-11-10")},
	}, &domain.ScopeAck{Date: d("2026-06-01"), BaselineCount: 2, LastTaskID: 3}, today)

	m, err := Migrate(ctx, path)
	if err != nil || m.From != 4 || m.To != 5 {
		t.Fatalf("Migrate = %+v, %v", m, err)
	}
	if _, ok := m.After["subtasks"]; ok || m.Before["subtasks"] != 4 || m.Before["journal_entries"] != 2 || m.After["journal_entries"] != 6 {
		t.Errorf("row counts: %+v", m)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// tasks follow the two existing entries in their old order, keeping dates and close times
	tasks, err := s.ListTasks(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	want := []domain.Task{
		{ID: 3, ActivityID: 1, Text: "kickoff", CreatedOn: d("2026-01-01"), ClosedOn: d("2026-02-01"), ClosedAt: "2026-02-01 10:15"},
		{ID: 4, ActivityID: 1, Text: "kickoff 2", CreatedOn: d("2026-01-01")},
		{ID: 5, ActivityID: 1, Text: "after ack", CreatedOn: d("2026-11-01"), ClosedOn: d("2026-11-05")},
		{ID: 6, ActivityID: 1, Text: "after ack 2", CreatedOn: d("2026-11-10")},
	}
	if len(tasks) != len(want) {
		t.Fatalf("tasks: %+v", tasks)
	}
	for i := range want {
		if tasks[i] != want[i] {
			t.Errorf("task %d = %+v, want %+v", i, tasks[i], want[i])
		}
	}
	entries, _ := s.ListJournal(ctx, 1)
	if len(entries) != 6 || entries[3].ID != 1 || entries[3].LoggedAt != "2026-01-01 09:00" || entries[2].Kind != domain.KindScopeAck {
		t.Errorf("journal: %+v", entries)
	}
	// the ack pointed at deleted task 3, so it now points at the last task before it
	ack, err := s.LatestScopeAck(ctx, 1)
	if err != nil || ack == nil || ack.LastTaskID != 4 {
		t.Fatalf("ack: %+v %v", ack, err)
	}
	if after := domain.ScopeCreep(act, tasks, ack, today); after.Added != before.Added || after.Total != before.Total ||
		after.GrowthPct != before.GrowthPct || after.Flagged != before.Flagged {
		t.Errorf("scope creep changed:\nbefore %+v\nafter  %+v", before, after)
	}
	// ids are never reused, even after deleting the newest entry
	if err := s.DeleteJournal(ctx, 6); err != nil {
		t.Fatal(err)
	}
	if next, err := s.AddTask(ctx, 1, "new", today); err != nil || next.ID != 7 {
		t.Errorf("next id = %d, %v", next.ID, err)
	}
}
