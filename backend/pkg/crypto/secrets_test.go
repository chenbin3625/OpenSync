package crypto

import "testing"

func TestEncryptDecryptSecretRoundTrip(t *testing.T) {
	key := "test-secret-key-material"
	plain := "alist-token-value"
	encrypted, err := EncryptSecret(plain, key)
	if err != nil {
		t.Fatalf("EncryptSecret() error: %v", err)
	}
	if encrypted == plain {
		t.Fatalf("EncryptSecret() did not encrypt value")
	}
	got, err := DecryptSecret(encrypted, key)
	if err != nil {
		t.Fatalf("DecryptSecret() error: %v", err)
	}
	if got != plain {
		t.Fatalf("DecryptSecret() = %q, want %q", got, plain)
	}
}

func TestMaybeDecryptSecretLegacyPlaintext(t *testing.T) {
	plain := "legacy-token"
	if got := MaybeDecryptSecret(plain, "key"); got != plain {
		t.Fatalf("MaybeDecryptSecret() = %q, want legacy plaintext", got)
	}
}
