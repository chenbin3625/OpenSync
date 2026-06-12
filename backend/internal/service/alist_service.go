package service

import (
	"context"
	"fmt"
	"log"
	"opensync/internal/i18n"
	"opensync/internal/mapper"
	"opensync/pkg/util"
	"strings"
	"sync"
)

var (
	alistClientList   = make(map[int64]*AlistClient)
	alistClientListMu sync.RWMutex
	alistClientLoads  = make(map[int64]*alistClientLoad)

	getAlistByID   = mapper.GetAlistByID
	newAlistClient = NewAlistClient
)

type alistClientLoad struct {
	wg     sync.WaitGroup
	client *AlistClient
	err    error
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
	alistClientListMu.RLock()
	client, ok := alistClientList[alistID]
	alistClientListMu.RUnlock()
	if ok {
		return client
	}

	load, owner := beginAlistClientLoad(alistID)
	if !owner {
		load.wg.Wait()
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

	newClient, err := newAlistClient(
		fmt.Sprintf("%v", alist["url"]),
		fmt.Sprintf("%v", alist["token"]),
		alistID,
	)
	if err != nil {
		finishAlistClientLoad(alistID, load, nil, err)
		panicAlistClientLoadError(err)
	}

	finishAlistClientLoad(alistID, load, newClient, nil)
	return newClient
}

func beginAlistClientLoad(alistID int64) (*alistClientLoad, bool) {
	alistClientListMu.Lock()
	defer alistClientListMu.Unlock()

	if client, ok := alistClientList[alistID]; ok {
		load := &alistClientLoad{client: client}
		load.wg.Add(1)
		load.wg.Done()
		return load, false
	}
	if load, ok := alistClientLoads[alistID]; ok {
		return load, false
	}

	load := &alistClientLoad{}
	load.wg.Add(1)
	alistClientLoads[alistID] = load
	return load, true
}

func finishAlistClientLoad(alistID int64, load *alistClientLoad, client *AlistClient, err error) {
	var previous *AlistClient
	alistClientListMu.Lock()
	load.client = client
	load.err = err
	if err == nil && client != nil {
		previous = alistClientList[alistID]
		alistClientList[alistID] = client
	}
	delete(alistClientLoads, alistID)
	alistClientListMu.Unlock()
	load.wg.Done()
	if previous != nil && previous != client {
		previous.Close()
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
	msg := i18n.G("add_alist_client_fail")
	msg = strings.Replace(msg, "{}", err.Error(), 1)
	panicPublic(msg)
}

func normalizeAlistInput(alist map[string]interface{}) string {
	remark, _ := alist["remark"]
	if remark != nil {
		if s, ok := remark.(string); ok && strings.TrimSpace(s) == "" {
			alist["remark"] = nil
		}
	}

	urlStr := strings.TrimRight(fmt.Sprintf("%v", alist["url"]), "/")
	alist["url"] = urlStr
	return urlStr
}

// UpdateClient updates an AList client
func UpdateClient(alist map[string]interface{}) {
	alistID := util.ToInt64(alist["id"])
	urlStr := normalizeAlistInput(alist)

	token, hasToken := alist["token"]
	if hasToken {
		if token == nil {
			delete(alist, "token")
			hasToken = false
		} else if s, ok := token.(string); ok {
			s = strings.TrimSpace(s)
			if s == "" {
				delete(alist, "token")
				hasToken = false
			} else {
				alist["token"] = s
			}
		}
	}

	alistOld, err := mapper.GetAlistByID(alistID)
	if err != nil {
		panic(err.Error())
	}

	oldURL := fmt.Sprintf("%v", alistOld["url"])
	if oldURL != urlStr || hasToken {
		if !hasToken {
			panicPublic(i18n.G("without_token"))
		}
		client, err := NewAlistClient(urlStr, fmt.Sprintf("%v", alist["token"]), alistID)
		if err != nil {
			panicPublic(err.Error())
		}
		storeAlistClient(alistID, client)
	}

	var tokenPtr *string
	if hasToken {
		t := fmt.Sprintf("%v", alist["token"])
		tokenPtr = &t
	}
	remarkStr := ""
	remark, _ := alist["remark"]
	if remark != nil {
		remarkStr = fmt.Sprintf("%v", remark)
	}
	if err := mapper.UpdateAlist(alistID, remarkStr, urlStr, tokenPtr); err != nil {
		panic(err.Error())
	}
}

// AddClient adds a new AList client
func AddClient(alist map[string]interface{}) {
	urlStr := normalizeAlistInput(alist)
	token := fmt.Sprintf("%v", alist["token"])

	client, err := NewAlistClient(urlStr, token, 0)
	if err != nil {
		log.Printf("Failed to add alist client: %v", err)
		panicPublic(err.Error())
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
		panicPublic(i18n.G("alist_in_use"))
	}

	removeCachedAlistClient(alistID)
	if err := mapper.RemoveAlist(alistID); err != nil {
		panic(err.Error())
	}
}

// GetChildPath gets child directory paths for path selector
func GetChildPath(ctx context.Context, alistID int64, path string) []map[string]string {
	client := GetClientByID(alistID)
	result, err := client.FilePathList(ctx, path)
	if err != nil {
		panicPublic(err.Error())
	}
	return result
}
