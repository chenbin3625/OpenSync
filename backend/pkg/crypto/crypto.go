package crypto

import (
	"crypto/rand"
	"fmt"
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

// isOwnerOnlySecretPerm reports whether perm grants no group, other, or execute
// permissions (e.g. 0400 read-only mounted secret or 0600 owner read/write).
func isOwnerOnlySecretPerm(perm os.FileMode) bool {
	return perm&^0o600 == 0
}

// ReadOrSetFile reads file content, creating it with defaultValue when it does
// not exist. Callers must treat a persist error as fatal when the value feeds
// cryptography: falling back to an in-memory default would silently rotate the
// secret on every restart and invalidate all stored cookies and tokens.
func ReadOrSetFile(fileName string, defaultVal string, force bool) (string, error) {
	if !force {
		if info, err := os.Lstat(fileName); err == nil {
			if !info.Mode().IsRegular() {
				return "", fmt.Errorf("%s is not a regular file", fileName)
			}
			data, err := os.ReadFile(fileName)
			if err != nil {
				return "", err
			}
			if len(strings.TrimSpace(string(data))) == 0 {
				return "", fmt.Errorf("%s is empty", fileName)
			}
			perm := info.Mode().Perm()
			if !isOwnerOnlySecretPerm(perm) {
				if err := os.Chmod(fileName, 0600); err != nil {
					return "", err
				}
				info, err = os.Stat(fileName)
				if err != nil {
					return "", err
				}
				perm = info.Mode().Perm()
			}
			if !isOwnerOnlySecretPerm(perm) {
				return "", fmt.Errorf("%s has insecure permissions %04o", fileName, perm)
			}
			return string(data), nil
		} else if !os.IsNotExist(err) {
			return "", err
		}
	}
	dir := filepath.Dir(fileName)
	if dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", err
		}
	}
	tmpFile, err := os.CreateTemp(dir, ".secret-*.tmp")
	if err != nil {
		return "", err
	}
	tmpName := tmpFile.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()

	if err := tmpFile.Chmod(0600); err != nil {
		_ = tmpFile.Close()
		return "", err
	}
	if _, err := tmpFile.WriteString(defaultVal); err != nil {
		_ = tmpFile.Close()
		return "", err
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return "", err
	}
	if err := tmpFile.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tmpName, fileName); err != nil {
		return "", err
	}
	cleanup = false

	info, err := os.Stat(fileName)
	if err != nil {
		return "", err
	}
	if !isOwnerOnlySecretPerm(info.Mode().Perm()) {
		return "", fmt.Errorf("%s has insecure permissions %04o", fileName, info.Mode().Perm())
	}
	return defaultVal, nil
}
