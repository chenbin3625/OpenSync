package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
)

const fileListPageWorkers = 8

type alistListRequest struct {
	Path    string `json:"path"`
	Refresh bool   `json:"refresh"`
	Page    int    `json:"page"`
	PerPage int    `json:"per_page"`
}

type alistPathRequest struct {
	Path string `json:"path"`
}

type alistRemoveRequest struct {
	Names []string `json:"names"`
	Dir   string   `json:"dir"`
}

type alistCopyMoveRequest struct {
	SrcDir    string   `json:"src_dir"`
	DstDir    string   `json:"dst_dir"`
	Overwrite bool     `json:"overwrite"`
	Names     []string `json:"names"`
}

func (c *AlistClient) FileListApiContext(ctx context.Context, path string, useCache int, scanInterval int) (FileListResult, error) {
	if err := c.CheckWaitContext(ctx, path, scanInterval); err != nil {
		return nil, err
	}

	req := alistListRequest{
		Path:    path,
		Refresh: useCache != 1,
		Page:    1,
		PerPage: fileListPageSize,
	}
	result := make(FileListResult, fileListPageSize)
	n, total, err := c.fetchFileListPage(ctx, req, result)
	if err != nil {
		return nil, err
	}
	if n == 0 || total <= n {
		return result, nil
	}

	// Some AList drivers clamp the page size below what was requested. When the
	// first page comes back short while more entries exist, the clamp is the
	// observed page size and the page count must be derived from it — deriving
	// pages from the requested size would silently drop every later page.
	listPageSize := fileListPageSize
	if n < fileListPageSize {
		listPageSize = n
	}
	if fileListLimitExceeded(len(result)) {
		return nil, fmt.Errorf("AList directory contains more than %d entries", maxFileListEntries)
	}
	// The first response reveals the total entry count. Reserve the final map
	// capacity once for multi-page listings so merging pages does not repeatedly
	// grow and rehash the result map.
	resultCapacity := total
	if resultCapacity > maxFileListEntries {
		resultCapacity = maxFileListEntries
	}
	if resultCapacity > len(result)*2 {
		resized := make(FileListResult, resultCapacity)
		for name, meta := range result {
			resized[name] = meta
		}
		result = resized
	}

	pages := (total + listPageSize - 1) / listPageSize
	if pages > maxFileListPages {
		return nil, fmt.Errorf("AList directory listing exceeded %d pages", maxFileListPages)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var mu sync.Mutex
	var fetchErr error
	// emptyPage records the first page that came back with zero entries. With an
	// accurate total that can only happen on the final page; an empty page
	// earlier means the server's total does not match its content, and merging
	// what arrived would silently drop files.
	emptyPage := 0
	var wg sync.WaitGroup
	fail := func(err error) {
		mu.Lock()
		if fetchErr == nil {
			fetchErr = err
			cancel()
		}
		mu.Unlock()
	}

	remainingPages := pages - 1
	workerCount := fileListPageWorkers
	if workerCount > remainingPages {
		workerCount = remainingPages
	}
	pageJobs := make(chan int)
	wg.Add(workerCount)
	for i := 0; i < workerCount; i++ {
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case page, ok := <-pageJobs:
					if !ok {
						return
					}
					if ctx.Err() != nil {
						return
					}

					pageReq := req
					pageReq.Page = page
					pageResult := make(FileListResult, fileListPageSize)
					fetched, _, err := c.fetchFileListPage(ctx, pageReq, pageResult)
					if err != nil {
						fail(err)
						return
					}
					mu.Lock()
					for name, meta := range pageResult {
						result[name] = meta
					}
					if fetched == 0 && page < pages && (emptyPage == 0 || page < emptyPage) {
						emptyPage = page
					}
					overCap := fileListLimitExceeded(len(result))
					mu.Unlock()
					if overCap {
						fail(fmt.Errorf("AList directory contains more than %d entries", maxFileListEntries))
						return
					}
				}
			}
		}()
	}

sendPages:
	for page := 2; page <= pages; page++ {
		select {
		case pageJobs <- page:
		case <-ctx.Done():
			break sendPages
		}
	}
	close(pageJobs)
	wg.Wait()
	if fetchErr != nil {
		return nil, fetchErr
	}
	// The parent context (task break/timeout) can cancel the workers between
	// requests; returning the partial map with a nil error would present an
	// incomplete directory listing as a successful one.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if emptyPage > 0 {
		return nil, fmt.Errorf("AList directory listing is incomplete: page %d of %d returned no entries (total=%d)", emptyPage, pages, total)
	}
	return result, nil
}

func fileListLimitExceeded(count int) bool {
	return count > maxFileListEntries
}

func (c *AlistClient) fetchFileListPage(ctx context.Context, req alistListRequest, result FileListResult) (int, int, error) {
	resp, err := c.startRequest(ctx, http.MethodPost, "/api/fs/list", req, nil)
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, 0, &alistStatusError{httpStatus: resp.StatusCode}
	}
	n, total, code, message, err := decodeFileListResponse(resp.Body, maxResponseBytes, result)
	if err != nil {
		return 0, 0, err
	}
	if err := c.checkAlistCode(code, message); err != nil {
		return 0, 0, err
	}
	return n, total, nil
}

func decodeFileListResponse(r io.Reader, limit int64, result FileListResult) (n, total, code int, message string, err error) {
	dec := json.NewDecoder(&capReader{r: r, limit: limit})
	if err = consumeDelim(dec, '{'); err != nil {
		return 0, 0, 0, "", err
	}
	for dec.More() {
		key, keyErr := decodeObjectKey(dec)
		if keyErr != nil {
			return 0, 0, 0, "", keyErr
		}
		switch key {
		case "code":
			if err = dec.Decode(&code); err != nil {
				return 0, 0, 0, "", err
			}
		case "message":
			if err = dec.Decode(&message); err != nil {
				return 0, 0, 0, "", err
			}
		case "data":
			n, total, err = decodeFileListData(dec, result)
			if err != nil {
				return 0, 0, 0, "", err
			}
		default:
			if err = skipJSONValue(dec); err != nil {
				return 0, 0, 0, "", err
			}
		}
	}
	if err = consumeDelim(dec, '}'); err != nil {
		return 0, 0, 0, "", err
	}
	return n, total, code, message, nil
}

func decodeFileListData(dec *json.Decoder, result FileListResult) (n, total int, err error) {
	tok, err := dec.Token()
	if err != nil {
		return 0, 0, err
	}
	if tok == nil {
		return 0, 0, nil
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '{' {
		return 0, 0, fmt.Errorf("AList list data is not an object")
	}
	for dec.More() {
		key, keyErr := decodeObjectKey(dec)
		if keyErr != nil {
			return 0, 0, keyErr
		}
		switch key {
		case "content":
			n, err = decodeFileListContent(dec, result)
			if err != nil {
				return 0, 0, err
			}
		case "total":
			if err = dec.Decode(&total); err != nil {
				return 0, 0, err
			}
		default:
			if err = skipJSONValue(dec); err != nil {
				return 0, 0, err
			}
		}
	}
	return n, total, consumeDelim(dec, '}')
}

func decodeFileListContent(dec *json.Decoder, result FileListResult) (int, error) {
	tok, err := dec.Token()
	if err != nil {
		return 0, err
	}
	if tok == nil {
		return 0, nil
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '[' {
		return 0, fmt.Errorf("AList list content is not an array")
	}
	n := 0
	for dec.More() {
		var entry FileListEntry
		if err := dec.Decode(&entry); err != nil {
			return n, err
		}
		addFileListEntry(result, entry)
		n++
	}
	return n, consumeDelim(dec, ']')
}

func addFileListEntry(result FileListResult, item FileListEntry) {
	if item.Name == "" {
		return
	}
	if item.IsDir {
		result[item.Name+"/"] = FileMetadata{}
		return
	}
	result[item.Name] = item.metadata()
}

func consumeDelim(dec *json.Decoder, want json.Delim) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	got, ok := tok.(json.Delim)
	if !ok || got != want {
		return fmt.Errorf("expected JSON delimiter %q, got %v", want, tok)
	}
	return nil
}

func decodeObjectKey(dec *json.Decoder) (string, error) {
	tok, err := dec.Token()
	if err != nil {
		return "", err
	}
	key, ok := tok.(string)
	if !ok {
		return "", fmt.Errorf("expected JSON object key, got %v", tok)
	}
	return key, nil
}

func skipJSONValue(dec *json.Decoder) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		for dec.More() {
			if _, err := dec.Token(); err != nil {
				return err
			}
			if err := skipJSONValue(dec); err != nil {
				return err
			}
		}
		return consumeDelim(dec, '}')
	case '[':
		for dec.More() {
			if err := skipJSONValue(dec); err != nil {
				return err
			}
		}
		return consumeDelim(dec, ']')
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delim)
	}
}
