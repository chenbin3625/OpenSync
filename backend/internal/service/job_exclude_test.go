package service

import "testing"

func TestParseExcludePatternsSupportsNewlinesAndComments(t *testing.T) {
	input := "# macOS\n.DS_Store\n\n._*\r\n# Windows\nThumbs.db\n"

	got := parseExcludePatterns(input)
	want := []string{"# macOS", ".DS_Store", "._*", "# Windows", "Thumbs.db"}

	if len(got) != len(want) {
		t.Fatalf("parseExcludePatterns() length = %d, want %d (%#v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("parseExcludePatterns()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestParseExcludePatternsKeepsColonCompatibility(t *testing.T) {
	got := parseExcludePatterns("*.tmp : .git/ : node_modules/")
	want := []string{"*.tmp", ".git/", "node_modules/"}

	if len(got) != len(want) {
		t.Fatalf("parseExcludePatterns() length = %d, want %d (%#v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("parseExcludePatterns()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestNormalizeExcludeStoresNewlineSeparatedRules(t *testing.T) {
	got := normalizeExclude("*.tmp : .git/ : node_modules/")
	want := "*.tmp\n.git/\nnode_modules/"

	if got != want {
		t.Fatalf("normalizeExclude() = %q, want %q", got, want)
	}
}

func TestExcludeMatchPathOmitsLeadingSlashForRootFiles(t *testing.T) {
	if got := excludeMatchPath("/src", "/src", "file.tmp"); got != "file.tmp" {
		t.Fatalf("excludeMatchPath(root file) = %q, want file.tmp", got)
	}
	if got := excludeMatchPath("/src", "/src/dir", "file.tmp"); got != "dir/file.tmp" {
		t.Fatalf("excludeMatchPath(nested file) = %q, want dir/file.tmp", got)
	}
}
