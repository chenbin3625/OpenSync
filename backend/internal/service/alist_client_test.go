package service

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

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

func TestGetClientByIDCoalescesConcurrentLoads(t *testing.T) {
	alistClientListMu.Lock()
	oldList := alistClientList
	alistClientList = make(map[int64]*AlistClient)
	alistClientListMu.Unlock()
	oldGet := getAlistByID
	oldNew := newAlistClient
	defer func() {
		alistClientListMu.Lock()
		alistClientList = oldList
		alistClientListMu.Unlock()
		getAlistByID = oldGet
		newAlistClient = oldNew
	}()

	var loads atomic.Int64
	getAlistByID = func(alistID int64) (map[string]interface{}, error) {
		return map[string]interface{}{
			"url":   "https://example.test",
			"token": "token",
		}, nil
	}
	newAlistClient = func(alistURL string, token string, alistID int64) (*AlistClient, error) {
		loads.Add(1)
		time.Sleep(20 * time.Millisecond)
		return &AlistClient{URL: alistURL, Token: token, AlistID: alistID}, nil
	}

	const workers = 16
	var wg sync.WaitGroup
	wg.Add(workers)
	clients := make([]*AlistClient, workers)
	for i := 0; i < workers; i++ {
		i := i
		go func() {
			defer wg.Done()
			clients[i] = GetClientByID(7)
		}()
	}
	wg.Wait()

	if loads.Load() != 1 {
		t.Fatalf("newAlistClient called %d times, want 1", loads.Load())
	}
	for i, client := range clients {
		if client == nil {
			t.Fatalf("clients[%d] = nil", i)
		}
		if client != clients[0] {
			t.Fatalf("clients[%d] = different pointer, want shared cached client", i)
		}
	}
}
