package crypto

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"math/big"
	"os"
	"strings"
)

const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// GeneratePassword generates a random password of given length
func GeneratePassword(length int) string {
	b := make([]byte, length)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		b[i] = charset[n.Int64()]
	}
	return string(b)
}

// PasswordToMD5 encrypts password with salt: MD5(password + secretKey)
func PasswordToMD5(passwd string, secretKey string) string {
	h := md5.New()
	h.Write([]byte(passwd + secretKey))
	return hex.EncodeToString(h.Sum(nil))
}

// ReadOrSetFile reads file content, creates with default if not exists
func ReadOrSetFile(fileName string, defaultVal string, force bool) string {
	if !force {
		if data, err := os.ReadFile(fileName); err == nil {
			return string(data)
		}
	}
	os.MkdirAll(strings.Join(strings.Split(fileName, "/")[:len(strings.Split(fileName, "/"))-1], "/"), 0755)
	os.WriteFile(fileName, []byte(defaultVal), 0644)
	return defaultVal
}
