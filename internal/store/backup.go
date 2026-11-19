package store

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
)

// Backup copies the database at srcPath to a new file at dest with VACUUM INTO.
// It never changes the source: the source must exist, is opened without
// applying migrations or setting its journal mode, and is only read, so a
// running server is neither blocked nor affected (SPEC §10 item 67). The
// destination must not exist and its directory must.
func Backup(ctx context.Context, srcPath, dest string) error {
	if _, err := os.Stat(srcPath); err != nil {
		return fmt.Errorf("backup source: %w", err)
	}
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("backup destination %s already exists; refusing to overwrite it", dest)
	}
	if info, err := os.Stat(filepath.Dir(dest)); err != nil || !info.IsDir() {
		return fmt.Errorf("backup destination directory %s does not exist", filepath.Dir(dest))
	}
	dsn := "file:" + (&url.URL{Path: srcPath}).EscapedPath() + "?mode=rw&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return fmt.Errorf("open %s: %w", srcPath, err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(ctx, `VACUUM INTO ?`, dest); err != nil {
		os.Remove(dest)
		return fmt.Errorf("backup to %s: %w", dest, err)
	}
	if err := os.Chmod(dest, 0o600); err != nil {
		return fmt.Errorf("backup %s: %w", dest, err)
	}
	return nil
}
