package service

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"opensync/internal/i18n"
	"opensync/internal/mapper"
)

var (
	alistClientList   = make(map[int64]*AlistClient)
	alistClientListMu sync.RWMutex
)

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

	alist, err := mapper.GetAlistByID(alistID)
	if err != nil {
		panic(err.Error())
	}

	newClient, err := NewAlistClient(
		fmt.Sprintf("%v", alist["url"]),
		fmt.Sprintf("%v", alist["token"]),
		alistID,
	)
	if err != nil {
		msg := i18n.G("add_alist_client_fail")
		msg = strings.Replace(msg, "{}", err.Error(), 1)
		panic(msg)
	}

	alistClientListMu.Lock()
	defer alistClientListMu.Unlock()
	if client, ok := alistClientList[alistID]; ok {
		return client
	}
	alistClientList[alistID] = newClient
	return newClient
}

// UpdateClient updates an AList client
func UpdateClient(alist map[string]interface{}) {
	alistID := toInt64(alist["id"])
	remark, _ := alist["remark"]
	if remark != nil {
		if s, ok := remark.(string); ok && strings.TrimSpace(s) == "" {
			alist["remark"] = nil
		}
	}

	urlStr := fmt.Sprintf("%v", alist["url"])
	if strings.HasSuffix(urlStr, "/") {
		urlStr = urlStr[:len(urlStr)-1]
		alist["url"] = urlStr
	}

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
			panic(i18n.G("without_token"))
		}
		client, err := NewAlistClient(urlStr, fmt.Sprintf("%v", alist["token"]), alistID)
		if err != nil {
			panic(err.Error())
		}
		alistClientListMu.Lock()
		alistClientList[alistID] = client
		alistClientListMu.Unlock()
	}

	var tokenPtr *string
	if hasToken {
		t := fmt.Sprintf("%v", alist["token"])
		tokenPtr = &t
	}
	remarkStr := ""
	if remark != nil {
		remarkStr = fmt.Sprintf("%v", remark)
	}
	if err := mapper.UpdateAlist(alistID, remarkStr, urlStr, tokenPtr); err != nil {
		panic(err.Error())
	}
}

// AddClient adds a new AList client
func AddClient(alist map[string]interface{}) {
	remark, _ := alist["remark"]
	if remark != nil {
		if s, ok := remark.(string); ok && strings.TrimSpace(s) == "" {
			alist["remark"] = nil
		}
	}

	urlStr := fmt.Sprintf("%v", alist["url"])
	if strings.HasSuffix(urlStr, "/") {
		urlStr = urlStr[:len(urlStr)-1]
		alist["url"] = urlStr
	}

	token := fmt.Sprintf("%v", alist["token"])

	client, err := NewAlistClient(urlStr, token, 0)
	if err != nil {
		log.Printf("Failed to add alist client: %v", err)
		panic(err.Error())
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
	alistClientListMu.Lock()
	alistClientList[newID] = client
	alistClientListMu.Unlock()
}

// RemoveClient removes an AList client
func RemoveClient(alistID int64) {
	alistClientListMu.Lock()
	delete(alistClientList, alistID)
	alistClientListMu.Unlock()
	mapper.RemoveAlist(alistID)
}

// GetChildPath gets child directory paths for path selector
func GetChildPath(alistID int64, path string) []map[string]string {
	client := GetClientByID(alistID)
	result, err := client.FilePathList(path)
	if err != nil {
		panic(err.Error())
	}
	return result
}
