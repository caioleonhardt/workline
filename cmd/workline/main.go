// Command workline is a local-first project tracker.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"workline/internal/domain"
	"workline/internal/store"
	"workline/internal/web"
)

const usage = `usage: workline <command> [flags]

commands:
  serve -db <path> [-addr 127.0.0.1:8080]   run the web app
  seed -db <path>   load the mockup sample data into an empty database
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
	case "seed":
		return runSeed(args[1:])
	default:
		fmt.Fprint(os.Stderr, usage)
		return fmt.Errorf("unknown command %q", args[0])
	}
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
	return store.Open(path)
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
	addr := fs.String("addr", "127.0.0.1:8080", "listen address (loopback only)")
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
