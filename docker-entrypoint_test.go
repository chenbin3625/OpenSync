package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
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

func TestPrepareSQLiteTempDirAllowsWritableDirOnChmodEPERM(t *testing.T) {
	tempDir := filepath.Join(t.TempDir(), "sqlite")
	if err := os.MkdirAll(tempDir, 0777); err != nil {
		t.Fatalf("mkdir tempDir: %v", err)
	}
	t.Setenv("SQLITE_TMPDIR", tempDir)
	t.Setenv("TMPDIR", "")

	err := prepareSQLiteTempDirWithChmod(func(string, fs.FileMode) error {
		return &os.PathError{Op: "chmod", Path: tempDir, Err: syscall.EPERM}
	})
	if err != nil {
		t.Fatalf("prepareSQLiteTempDirWithChmod() error = %v, want nil for writable dir under non-root", err)
	}
	if got := os.Getenv("SQLITE_TMPDIR"); got != tempDir {
		t.Fatalf("SQLITE_TMPDIR = %q, want %q", got, tempDir)
	}
}

func TestNumericEnvValidatesBounds(t *testing.T) {
	valid := []struct {
		raw  string
		want int
	}{
		{"", 1000},
		{"0", 0},
		{"1000", 1000},
		{"2147483647", 2147483647},
	}
	for _, tc := range valid {
		t.Setenv("TEST_UID", tc.raw)
		got, err := numericEnv("TEST_UID", 1000)
		if err != nil {
			t.Fatalf("numericEnv(%q) error = %v, want nil", tc.raw, err)
		}
		if got != tc.want {
			t.Fatalf("numericEnv(%q) = %d, want %d", tc.raw, got, tc.want)
		}
	}

	for _, raw := range []string{"-1", "+1000", "abc", "2147483648", "9999999999999"} {
		t.Setenv("TEST_UID", raw)
		if _, err := numericEnv("TEST_UID", 1000); err == nil {
			t.Fatalf("numericEnv(%q) error = nil, want error", raw)
		}
	}
}

func TestNonRootPrivilegeAndOwnershipDecisions(t *testing.T) {
	if !shouldManageOwnership(0) {
		t.Fatal("shouldManageOwnership(0) = false, want true")
	}
	if shouldManageOwnership(1000) {
		t.Fatal("shouldManageOwnership(1000) = true, want false")
	}
	if !shouldDropPrivileges(0, 1000, 1000) {
		t.Fatal("shouldDropPrivileges(0, 1000, 1000) = false, want true")
	}
	if shouldDropPrivileges(0, 0, 0) {
		t.Fatal("shouldDropPrivileges(0, 0, 0) = true, want false")
	}
	if shouldDropPrivileges(1000, 1000, 1000) {
		t.Fatal("shouldDropPrivileges(1000, 1000, 1000) = true, want false")
	}
}
