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

func TestFileChangedComparesSizeBeforeTrustingDigest(t *testing.T) {
	const digest = "0123456789abcdef0123456789abcdef"
	tests := []struct {
		name string
		src  FileMetadata
		dst  FileMetadata
		want bool
	}{
		{
			name: "differing size wins over identical digest",
			src:  FileMetadata{Size: 20, MD5: digest},
			dst:  FileMetadata{Size: 10, MD5: digest},
			want: true,
		},
		{
			name: "placeholder digest falls back to size",
			src:  FileMetadata{Size: 10, MD5: "unknown"},
			dst:  FileMetadata{Size: 10, MD5: "unknown"},
			want: false,
		},
		{
			name: "placeholder digest cannot mask a size change",
			src:  FileMetadata{Size: 20, MD5: "unknown"},
			dst:  FileMetadata{Size: 10, MD5: "unknown"},
			want: true,
		},
		{
			name: "same size different valid digest is a change",
			src:  FileMetadata{Size: 10, MD5: digest},
			dst:  FileMetadata{Size: 10, MD5: "fedcba9876543210fedcba9876543210"},
			want: true,
		},
		{
			name: "same size same digest is unchanged",
			src:  FileMetadata{Size: 10, MD5: digest},
			dst:  FileMetadata{Size: 10, MD5: digest},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fileChanged(tt.src, tt.dst); got != tt.want {
				t.Fatalf("fileChanged() = %v, want %v", got, tt.want)
			}
		})
	}
}
