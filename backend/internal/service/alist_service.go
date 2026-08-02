package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"opensync/internal/mapper"
	"opensync/internal/msg"
	"opensync/pkg/util"
	"strings"
	"sync"
)

var (
	alistClientList   = make(map[int64]*AlistClient)
	alistClientListMu sync.RWMutex
	alistClientLoads  = make(map[int64]*alistClientLoad)

	getAlistByID          = mapper.GetAlistByID
	newAlistClient        = NewAlistClient
	newAlistClientContext = NewAlistClientContext
)

type alistClientLoad struct {
	client   *AlistClient
	baseline *AlistClient
	err      error
	done     chan struct{}
}

// GetClientList returns all alist entries without token
func GetClientList() []map[string]interface{} {
	clientList, err := mapper.GetAlistList()
	if err != nil {
		panic(err.Error())
	}
	for _, client := range clientList {
		delete(client, "token")
	}
	return clientList
}

// GetClientByID gets or creates an AList client by ID
func GetClientByID(alistID int64) *AlistClient {
	return GetClientByIDContext(context.Background(), alistID)
}

func GetClientByIDContext(ctx context.Context, alistID int64) *AlistClient {
	if ctx == nil {
		ctx = context.Background()
	}
	alistClientListMu.RLock()
	client, ok := alistClientList[alistID]
	alistClientListMu.RUnlock()
	if ok {
		return client
	}

	load, owner := beginAlistClientLoad(alistID)
	if !owner {
		select {
		case <-load.done:
		case <-ctx.Done():
			panicAlistClientLoadError(ctx.Err())
		}
		if load.err != nil {
			panicAlistClientLoadError(load.err)
		}
		return load.client
	}

	alist, err := getAlistByID(alistID)
	if err != nil {
		finishAlistClientLoad(alistID, load, nil, err)
		panicAlistClientLoadError(err)
	}

	newClient, err := newAlistClientContext(
		ctx,
		fmt.Sprintf("%v", alist["url"]),
		fmt.Sprintf("%v", alist["token"]),
		alistID,
	)
	if err != nil {
		finishAlistClientLoad(alistID, load, nil, err)
		panicAlistClientLoadError(err)
	}

	finishAlistClientLoad(alistID, load, newClient, nil)
	return load.client
}

func beginAlistClientLoad(alistID int64) (*alistClientLoad, bool) {
	alistClientListMu.Lock()
	defer alistClientListMu.Unlock()

	if client, ok := alistClientList[alistID]; ok {
		load := &alistClientLoad{client: client, done: make(chan struct{})}
		close(load.done)
		return load, false
	}
	if load, ok := alistClientLoads[alistID]; ok {
		return load, false
	}

	load := &alistClientLoad{baseline: alistClientList[alistID], done: make(chan struct{})}
	alistClientLoads[alistID] = load
	return load, true
}

func finishAlistClientLoad(alistID int64, load *alistClientLoad, client *AlistClient, err error) {
	var stale *AlistClient
	alistClientListMu.Lock()
	load.err = err
	if err == nil && client != nil {
		current := alistClientList[alistID]
		if current != nil && current != load.baseline {
			// A newer client was stored (e.g. via UpdateClient) while we were
			// loading; keep the fresh one and discard the stale client we built.
			load.client = current
			stale = client
		} else {
			alistClientList[alistID] = client
			load.client = client
		}
	} else {
		load.client = client
	}
	delete(alistClientLoads, alistID)
	alistClientListMu.Unlock()
	close(load.done)
	if stale != nil {
		stale.Close()
	}
}

func storeAlistClient(alistID int64, client *AlistClient) {
	var previous *AlistClient
	alistClientListMu.Lock()
	previous = alistClientList[alistID]
	alistClientList[alistID] = client
	alistClientListMu.Unlock()
	if previous != nil && previous != client {
		previous.Close()
	}
}

func removeCachedAlistClient(alistID int64) {
	var previous *AlistClient
	alistClientListMu.Lock()
	previous = alistClientList[alistID]
	delete(alistClientList, alistID)
	alistClientListMu.Unlock()
	if previous != nil {
		previous.Close()
	}
}

func panicAlistClientLoadError(err error) {
	if err == nil {
		return
	}
	// Log the full error (which may include internal host/IP/port details) but
	// return only a generic message to the client so network topology is not
	// leaked through the API response.
	log.Printf("alist client load failed: %v", err)
	panicPublic(msg.AlistConnectFail)
}

func normalizeAlistInput(alist map[string]interface{}) string {
	remark, _ := alist["remark"]
	if remark != nil {
		if s, ok := remark.(string); ok && strings.TrimSpace(s) == "" {
			alist["remark"] = nil
		}
	}

	urlStr := strings.TrimRight(fmt.Sprintf("%v", alist["url"]), "/")
	if err := validateAlistURL(urlStr); err != nil {
		panicPublic(err.Error())
	}
	alist["url"] = urlStr
	return urlStr
}

func validateAlistURL(rawURL string) error {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return errors.New(msg.AlistURLInvalid)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme == "http" || scheme == "https" {
		return nil
	}
	return errors.New(msg.AlistURLInvalid)
}

func normalizeAlistToken(alist map[string]interface{}, required bool) (string, bool) {
	token, ok := alist["token"]
	if !ok || token == nil {
		if required {
			panicPublic(msg.AlistTokenRequired)
		}
		delete(alist, "token")
		return "", false
	}
	tokenStr := strings.TrimSpace(fmt.Sprintf("%v", token))
	if tokenStr == "" || tokenStr == "<nil>" {
		if required {
			panicPublic(msg.AlistTokenRequired)
		}
		delete(alist, "token")
		return "", false
	}
	alist["token"] = tokenStr
	return tokenStr, true
}

// UpdateClient updates an AList client
func UpdateClient(alist map[string]interface{}) {
	alistID := util.ToInt64(alist["id"])
	urlStr := normalizeAlistInput(alist)

	token, hasToken := normalizeAlistToken(alist, false)

	alistOld, err := mapper.GetAlistByID(alistID)
	if err != nil {
		panic(err.Error())
	}

	oldURL := fmt.Sprintf("%v", alistOld["url"])
	var client *AlistClient
	if oldURL != urlStr || hasToken {
		if !hasToken {
			panicPublic(msg.WithoutToken)
		}
		client, err = newAlistClient(urlStr, token, alistID)
		if err != nil {
			log.Printf("alist client update failed: %v", err)
			panicPublic(msg.AlistConnectFail)
		}
	}

	var tokenPtr *string
	if hasToken {
		tokenPtr = &token
	}
	remarkStr := ""
	remark, _ := alist["remark"]
	if remark != nil {
		remarkStr = fmt.Sprintf("%v", remark)
	}
	if err := mapper.UpdateAlist(alistID, remarkStr, urlStr, tokenPtr); err != nil {
		if client != nil {
			client.Close()
		}
		panic(err.Error())
	}
	if client != nil {
		storeAlistClient(alistID, client)
	}
}

// AddClient adds a new AList client
func AddClient(alist map[string]interface{}) {
	urlStr := normalizeAlistInput(alist)
	token, _ := normalizeAlistToken(alist, true)

	client, err := NewAlistClient(urlStr, token, 0)
	if err != nil {
		log.Printf("Failed to add alist client: %v", err)
		panicPublic(msg.AlistConnectFail)
	}

	remarkStr := ""
	if alist["remark"] != nil {
		remarkStr = fmt.Sprintf("%v", alist["remark"])
	}

	newID, err := mapper.AddAlist(remarkStr, urlStr, client.User, token)
	if err != nil {
		panic(err.Error())
	}

	client.AlistID = newID
	storeAlistClient(newID, client)
}

// RemoveClient removes an AList client
func RemoveClient(alistID int64) {
	count, err := mapper.CountJobsByAlistID(alistID)
	if err != nil {
		panic(err.Error())
	}
	if count > 0 {
		panicPublic(msg.AlistInUse)
	}

	removeCachedAlistClient(alistID)
	if err := mapper.RemoveAlist(alistID); err != nil {
		panic(err.Error())
	}
}

// GetChildPath gets child directory paths for path selector
func GetChildPath(ctx context.Context, alistID int64, path string) []map[string]string {
	client := GetClientByIDContext(ctx, alistID)
	result, err := client.FilePathList(ctx, path)
	if err != nil {
		log.Printf("alist path list failed: alistID=%d path=%q: %v", alistID, path, err)
		panicPublic(msg.AlistConnectFail)
	}
	return result
}
