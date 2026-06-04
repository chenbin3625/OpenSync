package service

import "testing"

func TestFileListEntryMetadataUsesHashInfoMD5(t *testing.T) {
	entry := FileListEntry{
		Name: "video.mkv",
		Size: 1024,
		HashInfo: map[string]interface{}{
			"md5": "ABCDEF0123456789",
		},
	}

	metadata := entry.metadata()

	if metadata.Size != 1024 {
		t.Fatalf("metadata.Size = %d, want 1024", metadata.Size)
	}
	if metadata.MD5 != "abcdef0123456789" {
		t.Fatalf("metadata.MD5 = %q, want lowercase md5", metadata.MD5)
	}
}

func TestFileListEntryMetadataParsesHashinfoString(t *testing.T) {
	entry := FileListEntry{
		Name:     "photo.jpg",
		Size:     2048,
		Hashinfo: `{"md5":"00112233445566778899aabbccddeeff"}`,
	}

	metadata := entry.metadata()

	if metadata.Size != 2048 {
		t.Fatalf("metadata.Size = %d, want 2048", metadata.Size)
	}
	if metadata.MD5 != "00112233445566778899aabbccddeeff" {
		t.Fatalf("metadata.MD5 = %q, want parsed md5", metadata.MD5)
	}
}
