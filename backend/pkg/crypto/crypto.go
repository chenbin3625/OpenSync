package crypto

import (
	"crypto/rand"
	"math/big"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

const (
	charset           = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	recoveryKeyLength = 24
)

// GeneratePassword generates a random password of given length
func GeneratePassword(length int) string {
	b := make([]byte, length)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			panic(err)
		}
		b[i] = charset[n.Int64()]
	}
	return string(b)
}

// GenerateRecoveryKey creates a one-time recovery key shown only to the user.
func GenerateRecoveryKey() string {
	return GeneratePassword(recoveryKeyLength)
}

// HashPassword creates a bcrypt password hash for newly stored passwords.
func HashPassword(passwd string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(passwd), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// CheckPassword validates modern bcrypt password hashes.
func CheckPassword(passwd string, storedHash string) bool {
	if !IsModernPasswordHash(storedHash) {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(passwd)) == nil
}

// IsModernPasswordHash reports whether a stored hash uses the current format.
func IsModernPasswordHash(storedHash string) bool {
	return strings.HasPrefix(storedHash, "$2a$") ||
		strings.HasPrefix(storedHash, "$2b$") ||
		strings.HasPrefix(storedHash, "$2y$")
}

// ReadOrSetFile reads file content, creating it with defaultValue when it does
// not exist. Callers must treat a persist error as fatal when the value feeds
// cryptography: falling back to an in-memory default would silently rotate the
// secret on every restart and invalidate all stored cookies and tokens.
func ReadOrSetFile(fileName string, defaultVal string, force bool) (string, error) {
	if !force {
		if data, err := os.ReadFile(fileName); err == nil && len(strings.TrimSpace(string(data))) > 0 {
			_ = os.Chmod(fileName, 0600)
			return string(data), nil
		}
	}
	dir := filepath.Dir(fileName)
	if dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", err
		}
	}
	if err := os.WriteFile(fileName, []byte(defaultVal), 0600); err != nil {
		return "", err
	}
	return defaultVal, nil
}
