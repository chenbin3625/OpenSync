package service

import "testing"

func TestNormalizeDirPathAddsTrailingSlash(t *testing.T) {
	got := normalizeDirPath("/google")
	if got != "/google/" {
		t.Fatalf("normalizeDirPath(/google) = %q, want %q", got, "/google/")
	}

	child := got + "camera/"
	if child != "/google/camera/" {
		t.Fatalf("child path = %q, want %q", child, "/google/camera/")
	}
}

func TestNormalizeDirPathKeepsRootAndExistingSlash(t *testing.T) {
	cases := map[string]string{
		"/":        "/",
		"/google/": "/google/",
		" /dst ":   "/dst/",
		"":         "",
	}

	for input, want := range cases {
		if got := normalizeDirPath(input); got != want {
			t.Fatalf("normalizeDirPath(%q) = %q, want %q", input, got, want)
		}
	}
}
