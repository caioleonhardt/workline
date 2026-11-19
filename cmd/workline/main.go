// Command workline is a local-first project tracker.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strconv"
	"syscall"
	"time"

	"workline/internal/domain"
	"workline/internal/store"
	"workline/internal/web"
)

// defaultAddr is where serve listens unless -addr or -port say otherwise.
const defaultAddr = "127.0.0.1:7357"

const usage = `usage: workline <command> [flags]

Every command works with no arguments, on the default database
($XDG_DATA_HOME/workline/workline.db, else ~/.local/share/workline/workline.db).

commands:
  serve    run the web app on ` + defaultAddr + `   [-addr host:port] [-port N] [-db path]
  backup   copy the database to a new file (the source is never changed)   [-db path] [dest]
  seed     load the mockup sample data into an empty database   [-db path]
  migrate  upgrade an existing database to this version, after backing it up   [-db path]
  version  print the version and the database schema it needs
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "workline:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return errors.New("missing command")
	}
	switch args[0] {
	case "serve":
		return runServe(args[1:])
	case "backup":
		return runBackup(args[1:])
	case "seed":
		return runSeed(args[1:])
	case "migrate":
		return runMigrateAt(args[1:], domain.SystemClock{})
	case "version", "-version", "--version":
		return runVersion(os.Stdout)
	default:
		fmt.Fprint(os.Stderr, usage)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

// version is this release; a release commit bumps it with CHANGELOG.md.
const version = "0.5.0"

// runVersion prints the release, the schema this build needs and, for a build
// from a git checkout, its revision.
func runVersion(w io.Writer) error {
	schema, err := store.LatestSchema()
	if err != nil {
		return err
	}
	info, _ := debug.ReadBuildInfo()
	fmt.Fprintf(w, "workline v%s (schema %d)%s\n", version, schema, revision(info))
	return nil
}

// revision returns " revision abc1234[, modified]" from the build's VCS
// stamp, or "" when there is none.
func revision(info *debug.BuildInfo) string {
	if info == nil {
		return ""
	}
	var rev, modified string
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value[:min(7, len(s.Value))]
		case "vcs.modified":
			if s.Value == "true" {
				modified = ", modified"
			}
		}
	}
	if rev == "" {
		return ""
	}
	return " revision " + rev + modified
}

// defaultDB is $XDG_DATA_HOME/workline/workline.db, else ~/.local/share/workline/workline.db.
func defaultDB() (string, error) {
	if x := os.Getenv("XDG_DATA_HOME"); x != "" {
		return filepath.Join(x, "workline", "workline.db"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find home directory: %w", err)
	}
	return filepath.Join(home, ".local", "share", "workline", "workline.db"), nil
}

// openStore creates the database's directory if needed and opens it.
func openStore(path string) (*store.Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}
	st, err := store.Open(path)
	if errors.Is(err, store.ErrMigrationNeeded) {
		return nil, fmt.Errorf("%w; run \"workline migrate\" (it backs the database up first)", err)
	}
	return st, err
}

func runSeed(args []string) error {
	def, err := defaultDB()
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("seed", flag.ContinueOnError)
	dbPath := fs.String("db", def, "path to the SQLite database")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("seed: unexpected argument %q", fs.Arg(0))
	}
	st, err := openStore(*dbPath)
	if err != nil {
		return err
	}
	defer st.Close()
	if err := st.Seed(context.Background()); err != nil {
		return err
	}
	fmt.Printf("seeded %s\n", *dbPath)
	return nil
}

func runBackup(args []string) error { return runBackupAt(args, domain.SystemClock{}) }

// runMigrateAt backs up an outdated database to the default backup file, then
// upgrades it in place. A current database is left alone.
func runMigrateAt(args []string, clock domain.Clock) error {
	def, err := defaultDB()
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("migrate", flag.ContinueOnError)
	dbPath := fs.String("db", def, "path to the SQLite database")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("migrate: unexpected argument %q", fs.Arg(0))
	}
	ctx := context.Background()
	have, want, err := store.SchemaVersions(ctx, *dbPath)
	if err != nil {
		return fmt.Errorf("migrate %s: %w", *dbPath, err)
	}
	if have == want {
		fmt.Printf("%s is up to date (schema %d)\n", *dbPath, want)
		return nil
	}
	if err := runBackupAt([]string{"-db", *dbPath}, clock); err != nil {
		return fmt.Errorf("migrate: backup first: %w", err)
	}
	m, err := store.Migrate(ctx, *dbPath)
	if err != nil {
		return fmt.Errorf("%w; the backup above holds the data as it was", err)
	}
	fmt.Printf("migrated %s: schema %d → %d\n", *dbPath, m.From, m.To)
	printRowCounts(os.Stdout, m.Before, m.After)
	return nil
}

// backupDest is the default backup file: <db directory>/backups/workline-YYYY-MM-DD-HHMM.db
// in local time. One backup per minute: a second one in the same minute is refused.
func backupDest(dbPath string, now time.Time) string {
	return filepath.Join(filepath.Dir(dbPath), "backups", "workline-"+now.Format("2006-01-02-1504")+".db")
}

func runBackupAt(args []string, clock domain.Clock) error {
	def, err := defaultDB()
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("backup", flag.ContinueOnError)
	dbPath := fs.String("db", def, "path to the SQLite database")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 1 {
		return errors.New("usage: workline backup [-db <path>] [dest]")
	}
	dest := fs.Arg(0)
	if dest == "" {
		if _, err := os.Stat(*dbPath); err != nil { // no stray folder for a wrong -db
			return fmt.Errorf("backup source: %w", err)
		}
		dest = backupDest(*dbPath, clock.Now())
		if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
			return fmt.Errorf("create backup directory: %w", err)
		}
		if _, err := os.Stat(dest); err == nil {
			return fmt.Errorf("%s already exists: only one backup per minute; try again in a minute or name a destination", dest)
		}
	}
	if err := store.Backup(context.Background(), *dbPath, dest); err != nil {
		return err
	}
	fmt.Printf("backed up %s to %s\n", *dbPath, dest)
	return nil
}

// printRowCounts lists each table's rows before → after a migration.
func printRowCounts(w io.Writer, before, after map[string]int) {
	var names []string
	for name := range before {
		names = append(names, name)
	}
	for name := range after {
		if _, ok := before[name]; !ok {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	fmt.Fprintf(w, "  %-16s %6s → %s\n", "table", "rows", "rows after")
	for _, name := range names {
		b, a := "-", "removed"
		if n, ok := before[name]; ok {
			b = strconv.Itoa(n)
		}
		if n, ok := after[name]; ok {
			a = strconv.Itoa(n)
		}
		fmt.Fprintf(w, "  %-16s %6s → %s\n", name, b, a)
	}
}

// resolveAddr applies -port (-1 when unset) to -addr and checks the result:
// a port in 1-65535 on a loopback host.
func resolveAddr(addr string, port int) (string, error) {
	host, p, err := net.SplitHostPort(addr)
	if err != nil {
		return "", fmt.Errorf("invalid -addr %q: %w", addr, err)
	}
	if port != -1 {
		p = strconv.Itoa(port)
	}
	if n, err := strconv.Atoi(p); err != nil || n < 1 || n > 65535 {
		return "", fmt.Errorf("invalid port %q: want a number from 1 to 65535", p)
	}
	addr = net.JoinHostPort(host, p)
	return addr, checkLoopback(addr)
}

// checkLoopback rejects listen addresses that are reachable from other machines.
func checkLoopback(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid -addr %q: %w", addr, err)
	}
	if ip := net.ParseIP(host); host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return fmt.Errorf("-addr %q: Workline listens on loopback addresses only", addr)
	}
	return nil
}

func runServe(args []string) error {
	def, err := defaultDB()
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	dbPath := fs.String("db", def, "path to the SQLite database")
	addr := fs.String("addr", defaultAddr, "listen address (loopback only)")
	port := fs.Int("port", -1, "port, overriding the one in -addr")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("serve: unexpected argument %q", fs.Arg(0))
	}
	listenAddr, err := resolveAddr(*addr, *port)
	if err != nil {
		return err
	}
	st, err := openStore(*dbPath)
	if err != nil {
		return err
	}
	defer st.Close()
	handler, err := web.New(st, domain.SystemClock{})
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return fmt.Errorf("cannot listen on %s: %w (is another program using the port? try -port N)", listenAddr, err)
	}
	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second,
		WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdown)
	}()
	slog.Info("listening", "url", "http://"+ln.Addr().String(), "db", *dbPath)
	if err := srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
