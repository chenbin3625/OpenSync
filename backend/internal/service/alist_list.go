package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
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
	// No capacity hint: Go maps with a hint allocate all buckets up front
	// (~57KB for 500 entries), and full-sync retains one listing per directory,
	// so small listings would otherwise waste hundreds of MB on large trees.
	result := make(FileListResult)
	n, total, err := c.fetchFileListPage(ctx, req, result)
	if err != nil {
		return nil, err
	}
	if fileListLimitExceeded(len(result)) || fileListLimitExceeded(n) {
		return nil, fmt.Errorf("AList directory contains more than %d entries", maxFileListEntries)
	}
	if total < 0 {
		return nil, fmt.Errorf("AList directory listing returned invalid total %d", total)
	}
	if n == 0 {
		if total > 0 {
			return nil, fmt.Errorf("AList directory listing is incomplete: first page returned no entries (total=%d)", total)
		}
		return result, nil
	}
	if total <= n {
		return result, nil
	}

	// Some AList drivers clamp the page size below what was requested. When the
	// first page comes back short while more entries exist, the clamp is the
	// observed page size and the page count must be derived from it — deriving
	// pages from the requested size would silently drop every later page.
	listPageSize := fileListPageSize
	if listPageSize <= 0 || n < listPageSize {
		listPageSize = n
	}

	// Overflow-safe ceiling division for positive total and listPageSize:
	// (total + listPageSize - 1) / listPageSize would overflow for large total.
	pages := (total-1)/listPageSize + 1
	if pages > maxFileListPages {
		return nil, fmt.Errorf("AList directory listing exceeded %d pages", maxFileListPages)
	}
	if fileListLimitExceeded(total) {
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

	remainingPages := pages - 1
	if remainingPages <= 0 {
		return result, nil
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var mu sync.Mutex
	var fetchErr error
	// fetchedTotal tracks raw entries decoded across all pages so dropping
	// unsafe names or deduplicating identical filenames does not falsely trip
	// completeness validation.
	fetchedTotal := n
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

	workerCount := fileListPageWorkers
	if workerCount > remainingPages {
		workerCount = remainingPages
	}
	pageResultCap := listPageSize
	if pageResultCap > maxFileListEntries {
		pageResultCap = maxFileListEntries
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

					// Only page 1 sends refresh: true; subsequent pages read the
					// refreshed snapshot so concurrent fetches do not hammer
					// AList's cache refresh or produce inconsistent pages.
					pageReq := req
					pageReq.Page = page
					pageReq.Refresh = false
					pageResult := make(FileListResult, pageResultCap)
					fetched, _, err := c.fetchFileListPage(ctx, pageReq, pageResult)
					if err != nil {
						fail(err)
						return
					}
					mu.Lock()
					for name, meta := range pageResult {
						result[name] = meta
					}
					fetchedTotal += fetched
					if fetched == 0 && (emptyPage == 0 || page < emptyPage) {
						emptyPage = page
					}
					overCap := fileListLimitExceeded(len(result)) || fileListLimitExceeded(fetchedTotal)
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
	if fetchedTotal < total {
		return nil, fmt.Errorf("AList directory listing is incomplete: received %d of %d entries", fetchedTotal, total)
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
	defer drainAndCloseBody(resp.Body)
	if resp.StatusCode != http.StatusOK {
		errLimit := maxResponseBytes
		if errLimit > 64<<10 {
			errLimit = 64 << 10
		}
		errBody, _ := readAllWithLimit(resp.Body, errLimit)
		return 0, 0, c.parseNonOKResponse(resp.StatusCode, errBody)
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
	var dataErr error
	hasData := false
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
			hasData = true
			var dErr error
			n, total, dErr, err = decodeFileListData(dec, result)
			if err != nil {
				return 0, 0, 0, "", err
			}
			if dErr != nil && dataErr == nil {
				dataErr = dErr
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
	if !hasData && dataErr == nil {
		dataErr = fmt.Errorf("AList list response missing data")
	}
	// If AList returned an API error code (e.g. 401/404/500) along with a
	// missing, null, or non-object data field, let checkAlistCode report
	// the API error instead of masking it with a shape error.
	if dataErr != nil && (code == 0 || code == http.StatusOK) {
		return 0, 0, code, message, dataErr
	}
	return n, total, code, message, nil
}

// decodeFileListData decodes the "data" field of /api/fs/list.
// shapeErr records non-fatal shape mismatches (e.g. data is null or a
// string/array on an error response) after skipping the full JSON value so the
// caller can keep reading code/message and invoke checkAlistCode. fatalErr
// indicates a stream, syntax, or limit error.
func decodeFileListData(dec *json.Decoder, result FileListResult) (n, total int, shapeErr, fatalErr error) {
	tok, err := dec.Token()
	if err != nil {
		return 0, 0, nil, err
	}
	if tok == nil {
		return 0, 0, fmt.Errorf("AList list data is null"), nil
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '{' {
		if err := skipJSONValueAfterToken(dec, tok); err != nil {
			return 0, 0, nil, err
		}
		return 0, 0, fmt.Errorf("AList list data is not an object"), nil
	}
	for dec.More() {
		key, keyErr := decodeObjectKey(dec)
		if keyErr != nil {
			return 0, 0, nil, keyErr
		}
		switch key {
		case "content":
			var cShapeErr error
			n, cShapeErr, err = decodeFileListContent(dec, result)
			if err != nil {
				return 0, 0, nil, err
			}
			if cShapeErr != nil && shapeErr == nil {
				shapeErr = cShapeErr
			}
		case "total":
			if err = dec.Decode(&total); err != nil {
				return 0, 0, nil, err
			}
		default:
			if err = skipJSONValue(dec); err != nil {
				return 0, 0, nil, err
			}
		}
	}
	return n, total, shapeErr, consumeDelim(dec, '}')
}

func decodeFileListContent(dec *json.Decoder, result FileListResult) (int, error, error) {
	tok, err := dec.Token()
	if err != nil {
		return 0, nil, err
	}
	if tok == nil {
		return 0, nil, nil
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '[' {
		if err := skipJSONValueAfterToken(dec, tok); err != nil {
			return 0, nil, err
		}
		return 0, fmt.Errorf("AList list content is not an array"), nil
	}
	n := 0
	for dec.More() {
		if n >= maxFileListEntries {
			return n, nil, fmt.Errorf("AList directory contains more than %d entries", maxFileListEntries)
		}
		var entry FileListEntry
		if err := dec.Decode(&entry); err != nil {
			return n, nil, err
		}
		addFileListEntry(result, entry)
		n++
	}
	return n, nil, consumeDelim(dec, ']')
}

func addFileListEntry(result FileListResult, item FileListEntry) {
	if !isSafeListedName(item.Name) {
		return
	}
	if item.IsDir {
		result[item.Name+"/"] = FileMetadata{}
		return
	}
	result[item.Name] = item.metadata()
}

// isSafeListedName rejects entry names that are not plain file names. Names come
// from the AList server, which may itself be proxying untrusted third-party
// storage, and they are concatenated onto directory paths when building scan
// work. A name carrying a separator or a parent reference would address a
// different directory than the one being scanned.
func isSafeListedName(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	return !strings.ContainsAny(name, "/\\")
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
	return skipJSONValueAfterToken(dec, tok)
}

func skipJSONValueAfterToken(dec *json.Decoder, tok json.Token) error {
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
