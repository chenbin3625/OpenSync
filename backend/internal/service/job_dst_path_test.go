package service

import "testing"

func TestParsePathListPreservesColonInJSONPaths(t *testing.T) {
	got := parsePathList(`["/movies/director:cut","/backup"]`)
	want := []string{"/movies/director:cut", "/backup"}

	if len(got) != len(want) {
		t.Fatalf("parsePathList() length = %d, want %d (%#v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("parsePathList()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestParsePathListTreatsColonAsPartOfSinglePath(t *testing.T) {
	got := parsePathList("/alpha:/beta")
	want := []string{"/alpha:/beta"}

	if len(got) != len(want) {
		t.Fatalf("parsePathList() length = %d, want %d (%#v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("parsePathList()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestCleanJobInputNormalizesDstPathToJSON(t *testing.T) {
	job := map[string]interface{}{
		"dstPath": []interface{}{"/movies/director:cut", " /backup "},
	}

	CleanJobInput(job)
	got := parsePathList(job["dstPath"])
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

func TestParsePathListPreservesColonInPaths(t *testing.T) {
	got := parsePathList(`["/movies/director:cut","/backup"]`)
	want := []string{"/movies/director:cut", "/backup"}

	if len(got) != len(want) {
		t.Fatalf("parsePathList() length = %d, want %d (%#v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("parsePathList()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestParsePathListTreatsPlainStringAsSinglePath(t *testing.T) {
	got := parsePathList("/movies/director:cut")
	want := []string{"/movies/director:cut"}

	if len(got) != len(want) {
		t.Fatalf("parsePathList() length = %d, want %d (%#v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("parsePathList()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestCleanJobInputNormalizesSrcPathToJSON(t *testing.T) {
	job := map[string]interface{}{
		"srcPath": []interface{}{"/photos", " /videos "},
	}

	CleanJobInput(job)
	got := parsePathList(job["srcPath"])
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

func TestValidateJobInputRejectsMissingRequiredFields(t *testing.T) {
	job := map[string]interface{}{
		"isCron": 2,
	}

	requirePublicPanic(t, func() {
		ValidateJobInput(job)
	})
}

func TestValidateJobInputAcceptsManualJobWithRequiredFields(t *testing.T) {
	job := map[string]interface{}{
		"srcPath": []string{"/src"},
		"dstPath": []string{"/dst"},
		"alistId": int64(1),
		"isCron":  2,
		"method":  0,
	}

	ValidateJobInput(job)
}

func TestDstPathForSrcSelectionPreservesSourceDirWhenMultipleSelected(t *testing.T) {
	got := dstPathForSrcSelection("/backup/", "/media/photos", []string{"/media/photos", "/archive/videos"})
	want := "/backup/photos/"

	if got != want {
		t.Fatalf("dstPathForSrcSelection() = %q, want %q", got, want)
	}
}

func TestDstPathForSrcSelectionKeepsSingleSourceAtTargetRoot(t *testing.T) {
	got := dstPathForSrcSelection("/backup/", "/media/photos", []string{"/media/photos"})
	want := "/backup/"

	if got != want {
		t.Fatalf("dstPathForSrcSelection() = %q, want %q", got, want)
	}
}

// Selecting sibling directories is how a partially checked tree branch arrives:
// the user ticked every child of one folder, so that folder is already
// accounted for by the selection itself and must not reappear on the
// destination. This matches the single-source layout, where `srcPaths` is not
// involved at all.
func TestDstPathForSrcSelectionDropsParentOfSiblingSelection(t *testing.T) {
	srcPaths := []string{"/drive/2", "/drive/学习"}
	tests := []struct {
		srcPath string
		want    string
	}{
		{srcPath: "/drive/2", want: "/backup/2/"},
		{srcPath: "/drive/学习", want: "/backup/学习/"},
	}

	for _, tt := range tests {
		got := dstPathForSrcSelection("/backup/", tt.srcPath, srcPaths)
		if got != tt.want {
			t.Errorf("dstPathForSrcSelection(%q) = %q, want %q", tt.srcPath, got, tt.want)
		}
	}
}

// Only the shared parent is subtracted. Intermediate directories below it still
// belong to the selection and have to survive, otherwise two branches with the
// same leaf name would collide at the destination root.
func TestDstPathForSrcSelectionPreservesNestedPathsBelowCommonParent(t *testing.T) {
	srcPaths := []string{"/drive/work/2", "/drive/study/notes"}
	tests := []struct {
		srcPath string
		want    string
	}{
		{srcPath: "/drive/work/2", want: "/backup/work/2/"},
		{srcPath: "/drive/study/notes", want: "/backup/study/notes/"},
	}

	for _, tt := range tests {
		got := dstPathForSrcSelection("/backup/", tt.srcPath, srcPaths)
		if got != tt.want {
			t.Errorf("dstPathForSrcSelection(%q) = %q, want %q", tt.srcPath, got, tt.want)
		}
	}
}

// A mixed selection is the case that regressed in v1.13.0: siblings plus nested
// paths share the parent of the siblings, and that parent must be stripped
// rather than pushed to the destination.
func TestDstPathForSrcSelectionStripsSharedParentOfMixedSelection(t *testing.T) {
	srcPaths := []string{"/nas/docker", "/nas/photos", "/nas/temp", "/nas/common/sec", "/nas/common/sync"}
	tests := []struct {
		srcPath string
		want    string
	}{
		{srcPath: "/nas/docker", want: "/google/docker/"},
		{srcPath: "/nas/photos", want: "/google/photos/"},
		{srcPath: "/nas/temp", want: "/google/temp/"},
		{srcPath: "/nas/common/sec", want: "/google/common/sec/"},
		{srcPath: "/nas/common/sync", want: "/google/common/sync/"},
	}

	for _, tt := range tests {
		got := dstPathForSrcSelection("/google/", tt.srcPath, srcPaths)
		if got != tt.want {
			t.Errorf("dstPathForSrcSelection(%q) = %q, want %q", tt.srcPath, got, tt.want)
		}
	}
}

// Identically named leaves in different branches must stay separated. Keeping
// the intermediate directories is what prevents them from colliding at the
// destination root.
func TestDstPathForSrcSelectionKeepsIdenticalLeavesApart(t *testing.T) {
	srcPaths := []string{"/drive/work/2", "/drive/study/2"}
	tests := []struct {
		srcPath string
		want    string
	}{
		{srcPath: "/drive/work/2", want: "/backup/work/2/"},
		{srcPath: "/drive/study/2", want: "/backup/study/2/"},
	}

	seen := make(map[string]string, len(tests))
	for _, tt := range tests {
		got := dstPathForSrcSelection("/backup/", tt.srcPath, srcPaths)
		if got != tt.want {
			t.Errorf("dstPathForSrcSelection(%q) = %q, want %q", tt.srcPath, got, tt.want)
		}
		if other, dup := seen[got]; dup {
			t.Errorf("srcPaths %q and %q both resolve to %q", other, tt.srcPath, got)
		}
		seen[got] = tt.srcPath
	}
}

// Sources with no shared parent fall back to the historical base-name layout.
func TestDstPathForSrcSelectionUsesBaseNameWithoutSharedParent(t *testing.T) {
	srcPaths := []string{"/media/photos", "/archive/videos"}
	tests := []struct {
		srcPath string
		want    string
	}{
		{srcPath: "/media/photos", want: "/backup/photos/"},
		{srcPath: "/archive/videos", want: "/backup/videos/"},
	}

	for _, tt := range tests {
		got := dstPathForSrcSelection("/backup/", tt.srcPath, srcPaths)
		if got != tt.want {
			t.Errorf("dstPathForSrcSelection(%q) = %q, want %q", tt.srcPath, got, tt.want)
		}
	}
}

func TestCommonSrcSelectionParentReturnsDeepestSharedDirectory(t *testing.T) {
	tests := []struct {
		name     string
		srcPaths []string
		want     string
	}{
		{name: "siblings", srcPaths: []string{"/drive/2", "/drive/学习"}, want: "/drive"},
		{name: "nested", srcPaths: []string{"/drive/work/2", "/drive/study/notes"}, want: "/drive"},
		{name: "mixed", srcPaths: []string{"/nas/temp", "/nas/common/sec"}, want: "/nas"},
		{name: "no shared parent", srcPaths: []string{"/media/photos", "/archive/videos"}, want: "/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := commonSrcSelectionParent(tt.srcPaths); got != tt.want {
				t.Errorf("commonSrcSelectionParent(%#v) = %q, want %q", tt.srcPaths, got, tt.want)
			}
		})
	}
}
