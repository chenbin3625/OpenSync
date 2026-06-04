package service

import "testing"

func TestFileChangedUsesMD5WhenBothSidesHaveMD5(t *testing.T) {
	src := FileMetadata{Size: 100, MD5: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	dst := FileMetadata{Size: 100, MD5: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}

	if !fileChanged(src, dst) {
		t.Fatalf("fileChanged() = false, want true for same size with different md5")
	}
}

func TestFileChangedFallsBackToSizeWhenEitherMD5Missing(t *testing.T) {
	src := FileMetadata{Size: 100, MD5: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	dst := FileMetadata{Size: 100}

	if fileChanged(src, dst) {
		t.Fatalf("fileChanged() = true, want false when md5 is missing and size matches")
	}

	dst.Size = 101
	if !fileChanged(src, dst) {
		t.Fatalf("fileChanged() = false, want true when md5 is missing and size differs")
	}
}

func TestFileSizeReturnsRawSizeForMetadata(t *testing.T) {
	size := fileSize(FileMetadata{Size: 4096, MD5: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"})

	if size != int64(4096) {
		t.Fatalf("fileSize() = %v, want 4096", size)
	}
}
