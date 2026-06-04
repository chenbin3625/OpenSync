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

func TestParseDstPathsTreatsColonAsPartOfSinglePath(t *testing.T) {
	got := parseDstPaths("/alpha:/beta")
	want := []string{"/alpha:/beta"}

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

func TestParseSrcPathsPreservesColonInPaths(t *testing.T) {
	got := parseSrcPaths(`["/movies/director:cut","/backup"]`)
	want := []string{"/movies/director:cut", "/backup"}

	if len(got) != len(want) {
		t.Fatalf("parseSrcPaths() length = %d, want %d (%#v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("parseSrcPaths()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestParseSrcPathsTreatsPlainStringAsSinglePath(t *testing.T) {
	got := parseSrcPaths("/movies/director:cut")
	want := []string{"/movies/director:cut"}

	if len(got) != len(want) {
		t.Fatalf("parseSrcPaths() length = %d, want %d (%#v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("parseSrcPaths()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestCleanJobInputNormalizesSrcPathToJSON(t *testing.T) {
	job := map[string]interface{}{
		"srcPath": []interface{}{"/photos", " /videos "},
	}

	CleanJobInput(job)
	got := parseSrcPaths(job["srcPath"])
	want := []string{"/photos", "/videos"}

	if len(got) != len(want) {
		t.Fatalf("normalized srcPath length = %d, want %d (%#v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("normalized srcPath[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestDstPathForSrcSelectionPreservesSourceDirWhenMultipleSelected(t *testing.T) {
	got := dstPathForSrcSelection("/backup/", "/media/photos", true)
	want := "/backup/photos/"

	if got != want {
		t.Fatalf("dstPathForSrcSelection() = %q, want %q", got, want)
	}
}

func TestDstPathForSrcSelectionKeepsSingleSourceAtTargetRoot(t *testing.T) {
	got := dstPathForSrcSelection("/backup/", "/media/photos", false)
	want := "/backup/"

	if got != want {
		t.Fatalf("dstPathForSrcSelection() = %q, want %q", got, want)
	}
}
