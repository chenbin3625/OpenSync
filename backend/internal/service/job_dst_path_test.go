package service

import "testing"

func TestParseDstPathsPreservesColonInJSONPaths(t *testing.T) {
	got := parseDstPaths(`["/movies/director:cut","/backup"]`)
	want := []string{"/movies/director:cut", "/backup"}

	if len(got) != len(want) {
		t.Fatalf("parseDstPaths() length = %d, want %d (%#v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("parseDstPaths()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestParseDstPathsKeepsLegacyColonSeparator(t *testing.T) {
	got := parseDstPaths("/alpha:/beta")
	want := []string{"/alpha", "/beta"}

	if len(got) != len(want) {
		t.Fatalf("parseDstPaths() length = %d, want %d (%#v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("parseDstPaths()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestCleanJobInputNormalizesDstPathToJSON(t *testing.T) {
	job := map[string]interface{}{
		"dstPath": []interface{}{"/movies/director:cut", " /backup "},
	}

	CleanJobInput(job)
	got := parseDstPaths(job["dstPath"])
	want := []string{"/movies/director:cut", "/backup"}

	if len(got) != len(want) {
		t.Fatalf("normalized dstPath length = %d, want %d (%#v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("normalized dstPath[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
