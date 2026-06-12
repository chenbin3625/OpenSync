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
	secretPath := filepath.Join(t.TempDir(), "nested", "secret.key")

	if got := ReadOrSetFile(secretPath, "secret-value", false); got != "secret-value" {
		t.Fatalf("ReadOrSetFile() = %q, want default secret", got)
	}

	info, err := os.Stat(secretPath)
	if err != nil {
		t.Fatalf("stat secret: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0600 {
		t.Fatalf("secret permissions = %o, want 0600", mode)
	}
}
