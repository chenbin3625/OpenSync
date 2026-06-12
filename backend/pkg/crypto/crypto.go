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

// ReadOrSetFile reads file content, creates with default if not exists
func ReadOrSetFile(fileName string, defaultVal string, force bool) string {
	if !force {
		if data, err := os.ReadFile(fileName); err == nil {
			_ = os.Chmod(fileName, 0600)
			return string(data)
		}
	}
	dir := filepath.Dir(fileName)
	if dir != "." {
		_ = os.MkdirAll(dir, 0755)
	}
	_ = os.WriteFile(fileName, []byte(defaultVal), 0600)
	return defaultVal
}
