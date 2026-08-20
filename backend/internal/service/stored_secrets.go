package service

import (
	"opensync/internal/config"
	"opensync/pkg/crypto"
	"strings"
)

func encryptStoredSecret(plaintext string) string {
	if strings.TrimSpace(plaintext) == "" {
		return ""
	}
	encrypted, err := crypto.EncryptSecret(plaintext, config.GetPasswordStr())
	if err != nil {
		panic(err.Error())
	}
	return encrypted
}

func decryptStoredSecret(stored string) string {
	return crypto.MaybeDecryptSecret(stored, config.GetPasswordStr())
}
