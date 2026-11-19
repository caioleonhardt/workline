package main

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"workline/internal/domain"
	"workline/internal/store"
	"workline/migrations"
)

func TestCheckLoopback(t *testing.T) {
	for addr, ok := range map[string]bool{
		"127.0.0.1:8080": true, "localhost:9": true, "[::1]:8080": true, "127.0.0.2:1": true,
		"0.0.0.0:8080": false, ":8080": false, "192.168.1.5:80": false, "example.com:80": false, "8080": false,
	} {
		if err := checkLoopback(addr); (err == nil) != ok {
			t.Errorf("checkLoopback(%q) = %v, want ok=%v", addr, err, ok)
		}
	}
}

func TestDefaultAddrAndUsage(t *testing.T) {
	if defaultAddr != "127.0.0.1:7357" {
		t.Errorf("defaultAddr = %q", defaultAddr)
	}
	if got, err := resolveAddr(defaultAddr, -1); err != nil || got != "127.0.0.1:7357" {
		t.Errorf("default address does not resolve: %q %v", got, err)
	}
	if got, err := resolveAddr(defaultAddr, 7400); err != nil || got != "127.0.0.1:7400" {
		t.Errorf("-port over the default: %q %v", got, err)
	}
	for _, want := range []string{"7357", "Every command works with no arguments", "serve ", "backup ", "seed "} {
		if !strings.Contains(usage, want) {
			t.Errorf("usage lacks %q", want)
		}
	}
	if strings.Contains(usage, "8080") {
		t.Error("usage still mentions 8080")
	}
}

func TestResolveAddr(t *testing.T) {
	for _, c := range []struct {
		addr string
		port int
		want string // empty: an error
	}{
		{"127.0.0.1:8080", -1, "127.0.0.1:8080"},
		{"127.0.0.1:8080", 8081, "127.0.0.1:8081"},
		{"localhost:8080", 9000, "localhost:9000"},
		{"[::1]:8080", 9000, "[::1]:9000"},
		{"127.0.0.1:8080", 65535, "127.0.0.1:65535"},
		{"127.0.0.1:8080", 0, ""},
		{"127.0.0.1:8080", 65536, ""},
		{"127.0.0.1:8080", -2, ""},
		{"127.0.0.1:0", -1, ""},
		{"127.0.0.1:http", -1, ""},
		{"0.0.0.0:8080", 8081, ""}, // -port does not lift the loopback rule
		{":8080", -1, ""},
		{"8080", 8081, ""},
	} {
		got, err := resolveAddr(c.addr, c.port)
		if (err == nil) != (c.want != "") || got != c.want && c.want != "" {
			t.Errorf("resolveAddr(%q, %d) = %q, %v; want %q", c.addr, c.port, got, err, c.want)
		}
	}
}

func TestRunBackupUsage(t *testing.T) {
	if err := runBackup([]string{"a", "b"}); err == nil {
		t.Error("two destinations accepted")
	}
}

// seedDefaultDB creates a seeded database at the default location under a temp XDG_DATA_HOME.
func seedDefaultDB(t *testing.T) (dataDir string) {
	t.Helper()
	xdg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdg)
	dataDir = filepath.Join(xdg, "workline")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(dataDir, "workline.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.Seed(context.Background()); err != nil {
		t.Fatal(err)
	}
	return dataDir
}

func clockAt(day string, hour, minute int) domain.Clock {
	d, err := domain.ParseDate(day)
	if err != nil {
		panic(err)
	}
	return domain.FixedClock{D: d, Hour: hour, Minute: minute}
}

func TestBackupWithoutArgumentsUsesTheDataDir(t *testing.T) {
	dataDir := seedDefaultDB(t)
	if err := runBackupAt(nil, clockAt("2026-11-19", 14, 5)); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dataDir, "backups", "workline-2026-11-19-1405.db")
	info, err := os.Stat(dest)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("backup file: %v %v", info, err)
	}
	if dir, _ := os.Stat(filepath.Dir(dest)); dir.Mode().Perm() != 0o700 {
		t.Errorf("backups folder mode = %v", dir.Mode().Perm())
	}
	b, err := store.Open(dest)
	if err != nil {
		t.Fatal(err)
	}
	if ps, _ := b.ListProjects(context.Background(), true); len(ps) != 4 {
		t.Errorf("backup has %d projects", len(ps))
	}
	b.Close() // opening it leaves -wal/-shm files until closed

	// the same minute again is refused and the first backup is kept; the next minute works
	err = runBackupAt(nil, clockAt("2026-11-19", 14, 5))
	if err == nil || !strings.Contains(err.Error(), "only one backup per minute") {
		t.Errorf("same minute: %v", err)
	}
	if err := runBackupAt(nil, clockAt("2026-11-19", 14, 6)); err != nil {
		t.Errorf("next minute: %v", err)
	}
	entries, _ := os.ReadDir(filepath.Join(dataDir, "backups"))
	if len(entries) != 2 {
		t.Errorf("backups: %v", entries)
	}
}

func TestBackupDefaultsFollowDbAndHome(t *testing.T) {
	// -db given: the backups folder sits next to that file
	seedDefaultDB(t)
	other := filepath.Join(t.TempDir(), "elsewhere", "my.db")
	os.MkdirAll(filepath.Dir(other), 0o700)
	st, err := store.Open(other)
	if err != nil {
		t.Fatal(err)
	}
	st.Close()
	if err := runBackupAt([]string{"-db", other}, clockAt("2026-01-02", 3, 4)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(other), "backups", "workline-2026-01-02-0304.db")); err != nil {
		t.Errorf("backup next to -db: %v", err)
	}

	// no XDG_DATA_HOME: ~/.local/share/workline
	home := t.TempDir()
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".local", "share", "workline")
	os.MkdirAll(dir, 0o700)
	st, err = store.Open(filepath.Join(dir, "workline.db"))
	if err != nil {
		t.Fatal(err)
	}
	st.Close()
	if err := runBackupAt(nil, clockAt("2026-05-06", 7, 8)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "backups", "workline-2026-05-06-0708.db")); err != nil {
		t.Errorf("backup under HOME: %v", err)
	}
}

func TestBackupExplicitDestinationAndMissingSource(t *testing.T) {
	dataDir := seedDefaultDB(t)
	dest := filepath.Join(t.TempDir(), "named.db")
	if err := runBackupAt([]string{dest}, clockAt("2026-11-19", 1, 2)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dest); err != nil {
		t.Errorf("explicit destination: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "backups")); err == nil {
		t.Error("an explicit destination must not create the default backups folder")
	}
	// a refusal to overwrite still applies
	if err := runBackupAt([]string{dest}, clockAt("2026-11-19", 1, 2)); err == nil {
		t.Error("existing explicit destination overwritten")
	}
	// a wrong -db creates neither a database nor a backups folder
	missing := filepath.Join(t.TempDir(), "typo", "x.db")
	if err := runBackupAt([]string{"-db", missing}, clockAt("2026-11-19", 1, 2)); err == nil {
		t.Error("missing source accepted")
	}
	if _, err := os.Stat(filepath.Dir(missing)); err == nil {
		t.Error("a missing source created directories")
	}
}

func TestMigrateBacksUpThenUpgrades(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "old.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	first, _ := migrations.FS.ReadFile("0001_init.sql")
	if _, err := db.Exec(string(first) + `; PRAGMA user_version = 1; INSERT INTO projects (name) VALUES ('P')`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if _, err := openStore(path); err == nil || !strings.Contains(err.Error(), "workline migrate") {
		t.Errorf("openStore on an old database: %v", err)
	}
	if err := runMigrateAt([]string{"-db", path}, clockAt("2026-11-19", 10, 0)); err != nil {
		t.Fatal(err)
	}
	if have, want, err := store.SchemaVersions(context.Background(), filepath.Join(dir, "backups", "workline-2026-11-19-1000.db")); err != nil || have != 1 || want == 1 {
		t.Errorf("backup should keep schema 1: %d %d %v", have, want, err)
	}
	st, err := openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if ps, _ := st.ListProjects(context.Background(), true); len(ps) != 1 {
		t.Errorf("projects after migrating: %v", ps)
	}
}

func TestMigrateUpToDate(t *testing.T) {
	dataDir := seedDefaultDB(t)
	if err := runMigrateAt(nil, clockAt("2026-11-19", 10, 0)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "backups")); err == nil {
		t.Error("an up-to-date database was backed up")
	}
}
