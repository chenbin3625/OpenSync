package service

import "testing"

func TestCanonicalFileNameUnescapesCompatibleEncodings(t *testing.T) {
	cases := map[string]string{
		"Q&amp;A.mp4":        "Q&A.mp4",
		"Q&#38;A.mp4":        "Q&A.mp4",
		"Q%26A.mp4":          "Q&A.mp4",
		"Q%2526A.mp4":        "Q&A.mp4",
		"clip%EF%BC%9AA.mp4": "clip:A.mp4",
		"clip\uFF1FA.mp4":    "clip?A.mp4",
	}

	for input, want := range cases {
		if got := canonicalFileName(input); got != want {
			t.Fatalf("canonicalFileName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestDstNameMatchIndexRejectsAmbiguousDestinationCanonicalNames(t *testing.T) {
	dst := FileListResult{
		"Q%26A.mp4":   FileMetadata{Size: 10},
		"Q&amp;A.mp4": FileMetadata{Size: 10},
	}
	src := newSrcNameMatchIndex(FileListResult{
		"Q&A.mp4": FileMetadata{Size: 10},
	})

	if _, _, ok := newDstNameMatchIndex(dst).find("Q&A.mp4", src); ok {
		t.Fatalf("find() matched ambiguous destination names, want no fuzzy match")
	}
}

func TestDstNameMatchIndexRejectsAmbiguousSourceCanonicalNames(t *testing.T) {
	dst := FileListResult{
		"Q&amp;A.mp4": FileMetadata{Size: 10},
	}
	src := newSrcNameMatchIndex(FileListResult{
		"Q&A.mp4":   FileMetadata{Size: 10},
		"Q%26A.mp4": FileMetadata{Size: 10},
	})

	if _, _, ok := newDstNameMatchIndex(dst).find("Q&A.mp4", src); ok {
		t.Fatalf("find() matched ambiguous source names, want no fuzzy match")
	}
}
