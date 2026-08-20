package main

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gin-gonic/gin"
)

func TestAssetsGroupServesHashedFilesImmutableAndGzipped(t *testing.T) {
	gin.SetMode(gin.TestMode)
	assetsDist := fstest.MapFS{
		"app-hash123.js":    &fstest.MapFile{Data: []byte(strings.Repeat("console.log('x');", 200))},
		"style-hash456.css": &fstest.MapFile{Data: []byte(strings.Repeat(".a{color:red}", 300))},
	}
	router := gin.New()
	router.Use(maybeCompress())
	assets := router.Group("/assets")
	assets.Use(cachedStaticFiles())
	assets.StaticFS("", http.FS(assetsDist))

	// 1) prefers gzip + immutable
	req := httptest.NewRequest(http.MethodGet, "/assets/app-hash123.js", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("js status=%d want 200", w.Code)
	}
	if got := w.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("Cache-Control=%q", got)
	}
	if got := w.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding=%q", got)
	}
	zr, _ := gzip.NewReader(w.Result().Body)
	body, _ := io.ReadAll(zr)
	zr.Close()
	if len(body) == 0 || !strings.Contains(string(body), "console.log") {
		t.Fatalf("decompressed js body invalid: %d bytes", len(body))
	}

	// 2) SPA fallback must NOT claim assets
	req2 := httptest.NewRequest(http.MethodGet, "/assets/missing-bundle.js", nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusNotFound {
		t.Fatalf("missing asset status=%d want 404", w2.Code)
	}
}
