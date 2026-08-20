package config

import (
	"crypto/subtle"
	"log"
	"opensync/pkg/crypto"
	"os"
	"strings"
)

const setupTokenFile = "setup.token"

// EnsureSetupToken returns the one-time setup token for first admin creation.
func EnsureSetupToken() string {
	path := DataPath(setupTokenFile)
	if data, err := os.ReadFile(path); err == nil {
		if token := strings.TrimSpace(string(data)); token != "" {
			_ = os.Chmod(path, 0600)
			return token
		}
	}
	token := crypto.GeneratePassword(32)
	dir := DataDir()
	_ = os.MkdirAll(dir, 0750)
	_ = os.WriteFile(path, []byte(token), 0600)
	log.Printf("OpenSync setup token (required to create the first admin): %s", token)
	return token
}

// ValidateSetupToken checks the setup token when the system is not yet initialized.
func ValidateSetupToken(provided string) bool {
	expected := EnsureSetupToken()
	return subtle.ConstantTimeCompare([]byte(strings.TrimSpace(provided)), []byte(expected)) == 1
}

// RemoveSetupToken deletes the setup token after successful initialization.
func RemoveSetupToken() {
	_ = os.Remove(DataPath(setupTokenFile))
}
