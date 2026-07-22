package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareSQLiteTempDirCreatesWritableStickyDirectory(t *testing.T) {
	tempDir := filepath.Join(t.TempDir(), "sqlite")
	t.Setenv("SQLITE_TMPDIR", tempDir)
	t.Setenv("TMPDIR", "")

	if err := prepareSQLiteTempDir(); err != nil {
		t.Fatalf("prepareSQLiteTempDir() error: %v", err)
	}

	info, err := os.Stat(tempDir)
	if err != nil {
		t.Fatalf("stat sqlite temp directory: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("sqlite temp path is not a directory: mode=%v", info.Mode())
	}
	if got := info.Mode().Perm(); got != 0777 {
		t.Fatalf("sqlite temp permissions = %04o, want 0777", got)
	}
	if info.Mode()&os.ModeSticky == 0 {
		t.Fatalf("sqlite temp mode = %v, want sticky bit", info.Mode())
	}
	if got := os.Getenv("SQLITE_TMPDIR"); got != tempDir {
		t.Fatalf("SQLITE_TMPDIR = %q, want %q", got, tempDir)
	}
	if got := os.Getenv("TMPDIR"); got != tempDir {
		t.Fatalf("TMPDIR = %q, want %q", got, tempDir)
	}
}

func TestPrepareSQLiteTempDirRejectsRelativePath(t *testing.T) {
	t.Setenv("SQLITE_TMPDIR", "relative/path")

	err := prepareSQLiteTempDir()
	if err == nil {
		t.Fatal("prepareSQLiteTempDir() error = nil, want relative-path error")
	}
	if !strings.Contains(err.Error(), "absolute path") {
		t.Fatalf("prepareSQLiteTempDir() error = %q, want absolute-path detail", err)
	}
}

func TestPrepareSQLiteTempDirPreservesExplicitTMPDIR(t *testing.T) {
	tempDir := filepath.Join(t.TempDir(), "sqlite")
	t.Setenv("SQLITE_TMPDIR", tempDir)
	t.Setenv("TMPDIR", "/custom/general-temp")

	if err := prepareSQLiteTempDir(); err != nil {
		t.Fatalf("prepareSQLiteTempDir() error: %v", err)
	}
	if got := os.Getenv("TMPDIR"); got != "/custom/general-temp" {
		t.Fatalf("TMPDIR = %q, want explicit value preserved", got)
	}
}
