package i18n

import (
	"os"
	"sync"
	"testing"
)

func TestSetLanguageReportsUnsupportedLanguageValue(t *testing.T) {
	err := SetLanguage("en_us")
	if err == nil {
		t.Fatalf("SetLanguage(en_us) error = nil, want unsupported language error")
	}
	if err.Error() != "no en_us" {
		t.Fatalf("SetLanguage(en_us) error = %q, want %q", err.Error(), "no en_us")
	}
}

func TestLanguageConcurrentAccessIsRaceFree(t *testing.T) {
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir(temp) error: %v", err)
	}
	defer os.Chdir(oldWD)

	sysLanguage = ""
	defer func() {
		sysLanguage = ""
	}()

	if err := SetLanguage("zh_cn"); err != nil {
		t.Fatalf("SetLanguage(zh_cn) error: %v", err)
	}

	langs := []string{"zh_cn", "eng"}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = GetLanguage()
			}
		}()
		go func(offset int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				if err := SetLanguage(langs[(offset+j)%len(langs)]); err != nil {
					t.Errorf("SetLanguage() error: %v", err)
				}
			}
		}(i)
	}
	wg.Wait()
}
