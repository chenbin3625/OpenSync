package service

import (
	"context"
	"database/sql"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"opensync/internal/mapper"
	"opensync/internal/msg"
)

func resetAlistClientCacheForTest(t *testing.T) func() {
	t.Helper()
	alistClientListMu.Lock()
	oldList := alistClientList
	oldLoads := alistClientLoads
	oldGen := alistClientGeneration
	alistClientList = make(map[int64]*AlistClient)
	alistClientLoads = make(map[int64]*alistClientLoad)
	alistClientGeneration = make(map[int64]uint64)
	alistClientListMu.Unlock()

	oldGet := getAlistByID
	oldNew := newAlistClient
	oldNewCtx := newAlistClientContext
	oldTimeout := alistClientLoadTimeout

	return func() {
		alistClientListMu.Lock()
		alistClientList = oldList
		alistClientLoads = oldLoads
		alistClientGeneration = oldGen
		alistClientListMu.Unlock()

		getAlistByID = oldGet
		newAlistClient = oldNew
		newAlistClientContext = oldNewCtx
		alistClientLoadTimeout = oldTimeout
	}
}

func TestGetClientByIDContextFirstCallerCancelDoesNotPoisonCoalescedWaiters(t *testing.T) {
	defer resetAlistClientCacheForTest(t)()

	loadStarted := make(chan struct{})
	releaseLoad := make(chan struct{})
	var loadCtxCanceled atomic.Bool
	var loadCalls atomic.Int32

	getAlistByID = func(alistID int64) (map[string]interface{}, error) {
		return map[string]interface{}{
			"url":   "https://alist.example.test",
			"token": "secret-token",
		}, nil
	}
	newAlistClientContext = func(ctx context.Context, alistURL, token string, alistID int64) (*AlistClient, error) {
		loadCalls.Add(1)
		close(loadStarted)
		select {
		case <-ctx.Done():
			loadCtxCanceled.Store(true)
			return nil, ctx.Err()
		case <-releaseLoad:
			return &AlistClient{URL: alistURL, Token: token, AlistID: alistID}, nil
		}
	}

	ctx1, cancel1 := context.WithCancel(context.Background())
	caller1Done := make(chan interface{}, 1)
	go func() {
		defer func() {
			caller1Done <- recover()
		}()
		_ = GetClientByIDContext(ctx1, 11)
	}()

	<-loadStarted

	// Join as a second coalesced caller with an uncanceled context.
	caller2Client := make(chan *AlistClient, 1)
	caller2Panic := make(chan interface{}, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				caller2Panic <- r
			}
		}()
		caller2Client <- GetClientByIDContext(context.Background(), 11)
	}()

	// Wait briefly so caller 2 is parked on <-load.done before caller 1 cancels.
	time.Sleep(20 * time.Millisecond)
	cancel1()

	select {
	case recovered := <-caller1Done:
		if recovered == nil {
			t.Fatal("caller 1 did not panic on cancellation")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("caller 1 did not return promptly after cancellation")
	}

	close(releaseLoad)

	select {
	case p := <-caller2Panic:
		t.Fatalf("caller 2 panicked after caller 1 canceled: %v", p)
	case client := <-caller2Client:
		if client == nil || client.AlistID != 11 {
			t.Fatalf("caller 2 got client = %#v, want valid client for ID 11", client)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("caller 2 timed out waiting for coalesced load")
	}

	if loadCtxCanceled.Load() {
		t.Fatal("shared load context was canceled by first caller")
	}
	if loadCalls.Load() != 1 {
		t.Fatalf("newAlistClientContext called %d times, want 1", loadCalls.Load())
	}
}

func TestGetClientByIDContextPanicInLoadDoesNotDeadlockSubsequentCallers(t *testing.T) {
	defer resetAlistClientCacheForTest(t)()

	var attempt atomic.Int32
	getAlistByID = func(alistID int64) (map[string]interface{}, error) {
		return map[string]interface{}{
			"url":   "https://alist.example.test",
			"token": "secret-token",
		}, nil
	}
	newAlistClientContext = func(ctx context.Context, alistURL, token string, alistID int64) (*AlistClient, error) {
		if attempt.Add(1) == 1 {
			panic("boom during client init")
		}
		return &AlistClient{URL: alistURL, Token: token, AlistID: alistID}, nil
	}

	var firstPanic interface{}
	func() {
		defer func() {
			firstPanic = recover()
		}()
		_ = GetClientByIDContext(context.Background(), 19)
	}()
	if firstPanic == nil {
		t.Fatal("first GetClientByIDContext() did not panic on init panic")
	}

	// Subsequent caller must not deadlock and must succeed with a fresh load.
	done := make(chan *AlistClient, 1)
	go func() {
		done <- GetClientByIDContext(context.Background(), 19)
	}()

	select {
	case client := <-done:
		if client == nil || client.AlistID != 19 {
			t.Fatalf("second GetClientByIDContext() = %#v, want valid client", client)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("subsequent GetClientByIDContext() deadlocked after panic in earlier load")
	}
}

func TestStale401EvictionDoesNotDeleteUpdatedClient(t *testing.T) {
	defer resetAlistClientCacheForTest(t)()

	oldTransport := &closeTrackingTransport{}
	oldClient := &AlistClient{
		AlistID: 99,
		URL:     "https://old.example.test",
		Token:   "expired-token",
		client:  &http.Client{Transport: oldTransport},
	}
	newTransport := &closeTrackingTransport{}
	newClient := &AlistClient{
		AlistID: 99,
		URL:     "https://new.example.test",
		Token:   "fresh-token",
		client:  &http.Client{Transport: newTransport},
	}

	storeAlistClient(99, oldClient)
	storeAlistClient(99, newClient)

	if err := oldClient.checkAlistCode(401, "token expired"); err == nil || err.Error() != msg.AlistUnAuth {
		t.Fatalf("oldClient.checkAlistCode(401) error = %v, want %q", err, msg.AlistUnAuth)
	}

	alistClientListMu.RLock()
	cached := alistClientList[99]
	alistClientListMu.RUnlock()
	if cached != newClient {
		t.Fatalf("cached client = %#v, want newClient preserved after stale 401", cached)
	}
	if newTransport.closed.Load() {
		t.Fatal("newClient transport was closed by stale client's 401 eviction")
	}

	// A 401 on the active client itself MUST evict and close it.
	if err := newClient.checkAlistCode(401, "new token expired"); err == nil || err.Error() != msg.AlistUnAuth {
		t.Fatalf("newClient.checkAlistCode(401) error = %v, want %q", err, msg.AlistUnAuth)
	}
	alistClientListMu.RLock()
	_, stillCached := alistClientList[99]
	alistClientListMu.RUnlock()
	if stillCached {
		t.Fatal("active client was not evicted on its own 401")
	}
	if !newTransport.closed.Load() {
		t.Fatal("active client transport was not closed on eviction")
	}
}

func TestUpdateClientDoesNotResurrectConcurrentlyDeletedAlist(t *testing.T) {
	defer resetAlistClientCacheForTest(t)()

	testDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error: %v", err)
	}
	defer testDB.Close()
	if _, err := testDB.Exec(`CREATE TABLE alist_list(
		id integer primary key autoincrement,
		remark text,
		url text UNIQUE,
		userName text,
		token text
	)`); err != nil {
		t.Fatalf("create alist_list: %v", err)
	}
	if _, err := testDB.Exec(`CREATE TABLE job(
		id integer primary key autoincrement,
		alistId integer
	)`); err != nil {
		t.Fatalf("create job: %v", err)
	}
	if _, err := testDB.Exec("INSERT INTO alist_list(id, remark, url, userName, token) VALUES (1, '', 'https://old.test', 'admin', 'old-token')"); err != nil {
		t.Fatalf("insert alist: %v", err)
	}
	restoreDB := mapper.SetDBForTest(testDB)
	defer restoreDB()

	validating := make(chan struct{})
	resumeUpdate := make(chan struct{})
	newTransport := &closeTrackingTransport{}
	newClient := &AlistClient{
		AlistID: 1,
		URL:     "https://updated.test",
		Token:   "new-token",
		client:  &http.Client{Transport: newTransport},
	}

	newAlistClient = func(alistURL, token string, alistID int64) (*AlistClient, error) {
		close(validating)
		<-resumeUpdate
		return newClient, nil
	}

	oldClient := &AlistClient{AlistID: 1, URL: "https://old.test"}
	storeAlistClient(1, oldClient)

	var wg sync.WaitGroup
	wg.Add(1)
	var updatePanic interface{}
	go func() {
		defer wg.Done()
		defer func() {
			updatePanic = recover()
		}()
		UpdateClient(map[string]interface{}{
			"id":     int64(1),
			"url":    "https://updated.test",
			"token":  "new-token",
			"remark": "updated",
		})
	}()

	<-validating
	// Concurrently delete the AList config while UpdateClient is validating.
	RemoveClient(1)
	close(resumeUpdate)
	wg.Wait()

	if updatePanic == nil {
		t.Fatal("UpdateClient() succeeded after concurrent RemoveClient(); want not-found panic")
	}
	alistClientListMu.RLock()
	cached, exists := alistClientList[1]
	alistClientListMu.RUnlock()
	if exists || cached != nil {
		t.Fatalf("deleted AList was resurrected in cache: %#v", cached)
	}
	if !newTransport.closed.Load() {
		t.Fatal("UpdateClient() did not close newly created client after concurrent delete")
	}
}
