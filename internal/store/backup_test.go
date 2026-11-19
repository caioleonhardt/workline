package store

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"workline/migrations"
)

func TestBackupCopiesALiveDatabase(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "live.db")
	s, err := Open(src)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Seed(ctx); err != nil { // the server keeps its connection (and WAL) open
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "copy.db")
	if err := Backup(ctx, src, dest); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(dest); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("backup file: %v %v", info, err)
	}
	b, err := Open(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	ps, _ := b.ListProjects(ctx, true)
	subs, _ := b.ListAllTasks(ctx)
	if len(ps) != 4 || len(subs) != 46 {
		t.Errorf("backup has %d projects, %d tasks", len(ps), len(subs))
	}
	// the server can still write while and after a backup
	if _, err := s.CreateProject(ctx, "later"); err != nil {
		t.Errorf("source locked after backup: %v", err)
	}
}

// dbSnapshot is what a backup must not change in the source.
type dbSnapshot struct {
	Version              int
	Schema               string
	Projects, Activities int
}

func snapshot(t *testing.T, path string) dbSnapshot {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var snap dbSnapshot
	db.QueryRow(`PRAGMA user_version`).Scan(&snap.Version)
	var parts []string
	rows, err := db.Query(`SELECT name, sql FROM sqlite_master ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var name, ddl sql.NullString
		rows.Scan(&name, &ddl)
		parts = append(parts, name.String+":"+ddl.String)
	}
	rows.Close()
	snap.Schema = strings.Join(parts, "\n")
	db.QueryRow(`SELECT count(*) FROM projects`).Scan(&snap.Projects)
	db.QueryRow(`SELECT count(*) FROM activities`).Scan(&snap.Activities)
	return snap
}

func TestBackupNeverMigratesOrChangesTheSource(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "old.db")
	db, err := sql.Open("sqlite", "file:"+src)
	if err != nil {
		t.Fatal(err)
	}
	first, _ := migrations.FS.ReadFile("0001_init.sql")
	if err := migrate(ctx, db, fstest.MapFS{"0001_init.sql": {Data: first}}); err != nil { // an older database
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO projects (id, name) VALUES (1, 'P')`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	before := snapshot(t, src)
	dest := filepath.Join(dir, "old-copy.db")
	if err := Backup(ctx, src, dest); err != nil {
		t.Fatal(err)
	}
	if before.Version != 1 || before.Projects != 1 {
		t.Fatalf("test setup: want version 1 with one project: %+v", before)
	}
	if after := snapshot(t, src); after != before {
		t.Errorf("source changed:\n--- before\n%+v\n--- after\n%+v", before, after)
	}
	if got := snapshot(t, dest); got != before { // the copy is as old as the source, not migrated
		t.Errorf("backup differs from the source:\n%+v", got)
	}
}

func TestBackupRefusals(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.db")
	s, err := Open(src)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// an existing destination is left untouched
	existing := filepath.Join(dir, "keep.db")
	os.WriteFile(existing, []byte("precious"), 0o600)
	if err := Backup(ctx, src, existing); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("existing destination: %v", err)
	}
	if b, _ := os.ReadFile(existing); string(b) != "precious" {
		t.Error("existing destination was changed")
	}
	// a missing destination directory
	if err := Backup(ctx, src, filepath.Join(dir, "nope", "x.db")); err == nil || !strings.Contains(err.Error(), "directory") {
		t.Errorf("missing directory: %v", err)
	}
	// a wrong source path must not create an empty database
	missing := filepath.Join(dir, "typo.db")
	if err := Backup(ctx, missing, filepath.Join(dir, "out.db")); err == nil {
		t.Error("missing source accepted")
	}
	for _, p := range []string{missing, filepath.Join(dir, "out.db")} {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("%s was created", p)
		}
	}
}
