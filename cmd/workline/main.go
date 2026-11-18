// Command workline is a local-first project tracker.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"workline/internal/store"
)

const usage = `usage: workline <command> [flags]

commands:
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
	if err := os.MkdirAll(filepath.Dir(*dbPath), 0o700); err != nil {
		return fmt.Errorf("create database directory: %w", err)
	}
	st, err := store.Open(*dbPath)
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
