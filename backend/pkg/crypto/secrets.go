package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"strings"
)

const secretPrefix = "enc:v1:"

func deriveSecretKey(keyMaterial string) []byte {
	sum := sha256.Sum256([]byte(keyMaterial))
	return sum[:]
}

// EncryptSecret encrypts plaintext with AES-GCM using keyMaterial (e.g. secret.key).
func EncryptSecret(plaintext, keyMaterial string) (string, error) {
	plaintext = strings.TrimSpace(plaintext)
	if plaintext == "" {
		return "", nil
	}
	block, err := aes.NewCipher(deriveSecretKey(keyMaterial))
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return secretPrefix + base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptSecret decrypts values written by EncryptSecret. Legacy plaintext passes through.
func DecryptSecret(stored, keyMaterial string) (string, error) {
	stored = strings.TrimSpace(stored)
	if stored == "" {
		return "", nil
	}
	if !strings.HasPrefix(stored, secretPrefix) {
		return stored, nil
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, secretPrefix))
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(deriveSecretKey(keyMaterial))
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("ciphertext too short")
	}
	nonce, ciphertext := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// MaybeDecryptSecret returns plaintext or the original stored value on failure.
func MaybeDecryptSecret(stored, keyMaterial string) string {
	plain, err := DecryptSecret(stored, keyMaterial)
	if err != nil {
		return stored
	}
	return plain
}
