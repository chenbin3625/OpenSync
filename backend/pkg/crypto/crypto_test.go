package crypto

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHashPasswordUsesBcryptAndVerifiesPassword(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword() error: %v", err)
	}
	if !strings.HasPrefix(hash, "$2") {
		t.Fatalf("hash = %q, want bcrypt hash", hash)
	}
	if !CheckPassword("correct horse battery staple", hash) {
		t.Fatalf("CheckPassword() = false, want true for bcrypt hash")
	}
	if CheckPassword("wrong password", hash) {
		t.Fatalf("CheckPassword() = true, want false for wrong password")
	}
}

func TestCheckPasswordRejectsNonBcryptHashes(t *testing.T) {
	for _, storedHash := range []string{
		"old-password",
		"5ebe2294ecd0e0f08eab7690d2a6ee69",
		"",
	} {
		if CheckPassword("old-password", storedHash) {
			t.Fatalf("CheckPassword(%q) = true, want false for non-bcrypt hash", storedHash)
		}
	}
}

func TestReadOrSetFileCreatesSecretWithOwnerOnlyPermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested")
	secretPath := filepath.Join(dir, "secret.key")

	got, err := ReadOrSetFile(secretPath, "secret-value", false)
	if err != nil {
		t.Fatalf("ReadOrSetFile() error: %v", err)
	}
	if got != "secret-value" {
		t.Fatalf("ReadOrSetFile() = %q, want default secret", got)
	}

	info, err := os.Stat(secretPath)
	if err != nil {
		t.Fatalf("stat secret: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0600 {
		t.Fatalf("secret permissions = %o, want 0600", mode)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "secret.key" {
		t.Fatalf("unexpected files left after atomic write: %v", entries)
	}
}

func TestReadOrSetFileReportsUnwritableLocation(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("setup blocker: %v", err)
	}
	// The parent path is a regular file, so creating the nested directory must
	// fail and the error must surface instead of returning the default value.
	if _, err := ReadOrSetFile(filepath.Join(blocker, "secret.key"), "secret-value", false); err == nil {
		t.Fatalf("ReadOrSetFile() = nil error, want persist failure")
	}
}

func TestReadOrSetFileReturnsExistingContent(t *testing.T) {
	secretPath := filepath.Join(t.TempDir(), "secret.key")
	if err := os.WriteFile(secretPath, []byte("existing"), 0o600); err != nil {
		t.Fatalf("setup secret: %v", err)
	}
	got, err := ReadOrSetFile(secretPath, "replacement", false)
	if err != nil {
		t.Fatalf("ReadOrSetFile() error: %v", err)
	}
	if got != "existing" {
		t.Fatalf("ReadOrSetFile() = %q, want existing content", got)
	}
}

func TestReadOrSetFileAllowsReadOnly0400SecretFile(t *testing.T) {
	secretPath := filepath.Join(t.TempDir(), "secret.key")
	if err := os.WriteFile(secretPath, []byte("mounted-secret"), 0o400); err != nil {
		t.Fatalf("setup secret: %v", err)
	}
	got, err := ReadOrSetFile(secretPath, "replacement", false)
	if err != nil {
		t.Fatalf("ReadOrSetFile() error = %v, want nil for 0400 read-only secret", err)
	}
	if got != "mounted-secret" {
		t.Fatalf("ReadOrSetFile() = %q, want mounted-secret", got)
	}
	info, err := os.Stat(secretPath)
	if err != nil {
		t.Fatalf("stat secret: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o400 {
		t.Fatalf("0400 secret mode changed to %04o, want 0400 preserved without chmod", mode)
	}
}

func TestReadOrSetFileTightensInsecurePermissions(t *testing.T) {
	secretPath := filepath.Join(t.TempDir(), "secret.key")
	if err := os.WriteFile(secretPath, []byte("loose-secret"), 0o644); err != nil {
		t.Fatalf("setup secret: %v", err)
	}
	got, err := ReadOrSetFile(secretPath, "replacement", false)
	if err != nil {
		t.Fatalf("ReadOrSetFile() error = %v", err)
	}
	if got != "loose-secret" {
		t.Fatalf("ReadOrSetFile() = %q, want loose-secret", got)
	}
	info, err := os.Stat(secretPath)
	if err != nil {
		t.Fatalf("stat secret: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("tightened mode = %04o, want 0600", mode)
	}
}
