package main

import (
	"compress/gzip"
	"context"
	"crypto/tls"
	"embed"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"opensync/internal/config"
	"opensync/internal/handler"
	"opensync/internal/mapper"
	"opensync/internal/middleware"
	"opensync/internal/model"
	"opensync/internal/service"
	"os"
	"os/signal"
	pathpkg "path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/quic-go/quic-go/http3"
)

//go:embed all:web
var webFS embed.FS

// gzipWriterPool reuses BestSpeed writers. Level 1 is the right default for
// already-minified JS/CSS and small JSON APIs: NAS CPUs spend far more time in
// DefaultCompression (level 6) than they save on the wire, and pooled writers
// keep per-request allocs off the hot path.
var gzipWriterPool = sync.Pool{
	New: func() any {
		w, err := gzip.NewWriterLevel(io.Discard, gzip.BestSpeed)
		if err != nil {
			panic(err)
		}
		return w
	},
}

// gzipResponseWriter transparently compresses the response body when the
// client advertises gzip support. Compression only kicks in for bodies we can
// meaningfully compress (static assets + JSON API responses); the SSE progress
// stream is explicitly excluded because EventSource clients cannot recover a
// burst-compressed stream (it also writes partial frames on every flush).
//
// gin's StaticFS (http.FileServer) sets Content-Length and honors Range
// requests, both incompatible with a transformed body, so the wrapper strips
// Content-Length (letting net/http switch to chunked encoding) and ranged
// requests are left uncompressed.
type gzipResponseWriter struct {
	gin.ResponseWriter
	gz    *gzip.Writer
	on    sync.Once
	wrote bool
}

// activate applies the gzip headers exactly once, before the first byte of the
// body is written, and drops Content-Length so the real body (now smaller)
// never mismatches the declared length.
func (g *gzipResponseWriter) activate() {
	g.on.Do(func() {
		w := gzipWriterPool.Get().(*gzip.Writer)
		w.Reset(g.ResponseWriter)
		g.gz = w
		g.Header().Set("Content-Encoding", "gzip")
		g.Header().Add("Vary", "Accept-Encoding")
		g.Header().Del("Content-Length")
	})
}

func (g *gzipResponseWriter) WriteHeader(code int) {
	// Bodyless statuses (204/304/1xx) must not carry Content-Encoding; the
	// gzip stream is only finalized when an actual body was written.
	if code >= 200 && code != http.StatusNoContent && code != http.StatusNotModified {
		g.activate()
	}
	g.ResponseWriter.WriteHeader(code)
}

func (g *gzipResponseWriter) Write(data []byte) (int, error) {
	g.activate()
	g.wrote = true
	return g.gz.Write(data)
}

func (g *gzipResponseWriter) WriteString(s string) (int, error) {
	return g.Write([]byte(s))
}

// Flush flushes the gzip buffer. gin buffers headers/body differently for
// streaming endpoints, and the SSE endpoint is excluded from compression, so
// a full flush (rather than buffering until Close) keeps incremental output
// flowing for any non-SSE streaming response.
func (g *gzipResponseWriter) Flush() {
	g.activate()
	_ = g.gz.Flush()
	g.ResponseWriter.Flush()
}

// Unwrap lets Go 1.20+ ResponseController reach the underlying writer for
// features such as Flush and EnableFullDuplex.
func (g *gzipResponseWriter) Unwrap() http.ResponseWriter {
	return g.ResponseWriter
}

// Pusher forwards HTTP/2 server push when the underlying writer supports it.
func (g *gzipResponseWriter) Pusher() (pusher http.Pusher) {
	if p, ok := g.ResponseWriter.(http.Pusher); ok {
		return p
	}
	return nil
}

// closeCompression finalizes the gzip stream after the handler completes
// and returns the writer to the pool. Writers that never produced a body are
// reset without Close() so we do not emit an empty gzip footer.
func (g *gzipResponseWriter) closeCompression() {
	if g.gz != nil {
		if g.wrote {
			_ = g.gz.Close()
		}
		g.gz.Reset(io.Discard)
		gzipWriterPool.Put(g.gz)
		g.gz = nil
	}
	g.ResponseWriter.Flush()
}

// acceptsGzip reports whether the request advertises gzip support without an
// explicit q=0 veto (q values below 0.01 are treated as "not accepted").
func acceptsGzip(r *http.Request) bool {
	for _, part := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
		enc := strings.TrimSpace(part)
		if enc == "" {
			continue
		}
		name, _, _ := strings.Cut(enc, ";")
		if !strings.EqualFold(strings.TrimSpace(name), "gzip") {
			continue
		}
		for _, param := range strings.Split(enc, ";")[1:] {
			kv := strings.SplitN(strings.TrimSpace(param), "=", 2)
			if len(kv) == 2 && strings.EqualFold(kv[0], "q") {
				if q, err := strconv.ParseFloat(kv[1], 64); err == nil && q <= 0 {
					return false
				}
			}
		}
		return true
	}
	return false
}

// maybeCompress wraps responses in gzip when the request accepts it and the
// endpoint is compressible. Static assets and JSON APIs are the main wins.
func maybeCompress() gin.HandlerFunc {
	return func(c *gin.Context) {
		// The progress stream must stay raw: EventSource has no gzip support
		// and partial-frame flushes make a compressed stream unreadable.
		// Range requests are also left alone: a partial response cannot be
		// meaningfully gzipped without breaking the HTTP range contract.
		if c.Request.URL.Path == "/svr/job/stream" ||
			!acceptsGzip(c.Request) ||
			c.Request.Header.Get("Range") != "" {
			c.Next()
			return
		}
		w := &gzipResponseWriter{ResponseWriter: c.Writer}
		c.Writer = w
		c.Next()
		w.closeCompression()
	}
}

// cachedStaticFiles sets long-lived "immutable" cache headers for the
// content-hashed /assets/* bundle emitted by Vite. The file name is a hash of
// its bytes, so an unchanged URL can never serve stale content — exactly the
// case where immutable caching is both safe and the biggest load win.
func cachedStaticFiles() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead {
			c.Header("Cache-Control", "public, max-age=31536000, immutable")
		}
		c.Next()
	}
}

// noCacheHTML keeps the SPA shell (index.html) always fresh so clients pick
// up new bundle hashes, while the hashed assets themselves stay immutable.
func noCacheHTML() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-cache")
		c.Next()
	}
}

// errorRecovery catches panics and returns them as 500 responses
func errorRecovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Recovered panic: %v", r)
				if err, ok := r.(model.PublicError); ok {
					c.JSON(http.StatusInternalServerError, model.Error(err.Error()))
					c.Abort()
					return
				}
				c.JSON(http.StatusInternalServerError, model.Error("internal server error"))
				c.Abort()
			}
		}()
		c.Next()
	}
}

// maxRequestBodySize bounds JSON/form request bodies so an authenticated user
// cannot exhaust memory with an oversized payload. The app does not accept file
// uploads through HTTP (syncing happens via the AList API), so 1MB is generous.
func maxRequestBodySize(max int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, max)
		}
		c.Next()
	}
}

func securityHeaders(http3Port int) gin.HandlerFunc {
	altSvc := ""
	if http3Port > 0 {
		altSvc = fmt.Sprintf(`h3=":%d"; ma=86400`, http3Port)
	}
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Referrer-Policy", "no-referrer")
		c.Header("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		c.Header("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		if altSvc != "" {
			c.Header("Alt-Svc", altSvc)
		}
		c.Next()
	}
}

func serveWebFile(c *gin.Context, webDist fs.FS, filePath, contentType string) {
	newCachedWebFS(webDist).serve(c, filePath, contentType)
}

type cachedWebFS struct {
	fs    fs.FS
	mu    sync.RWMutex
	files map[string][]byte
}

func newCachedWebFS(webDist fs.FS) *cachedWebFS {
	return &cachedWebFS{fs: webDist, files: make(map[string][]byte)}
}

func (cache *cachedWebFS) warm(paths ...string) {
	for _, filePath := range paths {
		_, _ = cache.read(filePath)
	}
}

func (cache *cachedWebFS) read(filePath string) ([]byte, error) {
	cache.mu.RLock()
	data, ok := cache.files[filePath]
	cache.mu.RUnlock()
	if ok {
		return data, nil
	}

	data, err := fs.ReadFile(cache.fs, filePath)
	if err != nil {
		return nil, err
	}

	cache.mu.Lock()
	if existing, ok := cache.files[filePath]; ok {
		cache.mu.Unlock()
		return existing, nil
	}
	cache.files[filePath] = data
	cache.mu.Unlock()
	return data, nil
}

func (cache *cachedWebFS) serve(c *gin.Context, filePath, contentType string) {
	data, err := cache.read(filePath)
	if err != nil {
		c.String(http.StatusNotFound, "Frontend not found")
		return
	}
	c.Data(http.StatusOK, contentType, data)
}

func serveSPAFallback(c *gin.Context, webDist fs.FS) {
	serveSPAFallbackWithCache(c, newCachedWebFS(webDist))
}

func serveSPAFallbackWithCache(c *gin.Context, webFiles *cachedWebFS) {
	requestPath := c.Request.URL.Path
	if c.Request.Method != http.MethodGet ||
		requestPath == "/svr" ||
		strings.HasPrefix(requestPath, "/svr/") ||
		strings.HasPrefix(requestPath, "/assets/") ||
		pathpkg.Ext(requestPath) != "" {
		c.Status(http.StatusNotFound)
		return
	}
	webFiles.serve(c, "index.html", "text/html; charset=utf-8")
}

func main() {
	handled, err := runCLI(os.Args[1:], os.Stdout)
	if handled {
		if err != nil {
			log.Fatalf("Command failed: %v", err)
		}
		return
	}
	if err := run(context.Background()); err != nil {
		log.Fatalf("Server stopped: %v", err)
	}
}

func runCLI(args []string, stdout io.Writer) (bool, error) {
	if len(args) == 0 {
		return false, nil
	}
	switch args[0] {
	case "reset-password":
		fs := flag.NewFlagSet("reset-password", flag.ContinueOnError)
		fs.SetOutput(stdout)
		userName := fs.String("user", "", "user name")
		if err := fs.Parse(args[1:]); err != nil {
			return true, err
		}
		if strings.TrimSpace(*userName) == "" {
			return true, errors.New("missing --user")
		}
		return true, runResetPasswordCommand(*userName, stdout)
	case "healthcheck":
		return true, runHealthCheckCommand()
	default:
		return false, nil
	}
}

// runHealthCheckCommand performs an HTTP GET on the server's own root URL and
// exits non-zero on failure. It touches neither the database nor the config
// files, so it is safe to run from the container HEALTHCHECK as any user.
func runHealthCheckCommand() error {
	port := os.Getenv("OPENSYNC_PORT")
	if port == "" {
		port = "8023"
	}
	scheme := "http"
	client := &http.Client{Timeout: 5 * time.Second}
	if strings.TrimSpace(os.Getenv("OPENSYNC_TLS_CERT")) != "" {
		scheme = "https"
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}
	resp, err := client.Get(scheme + "://127.0.0.1:" + port + "/")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusBadRequest {
		return nil
	}
	return fmt.Errorf("healthcheck: unexpected status %d", resp.StatusCode)
}

func runResetPasswordCommand(userName string, stdout io.Writer) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("%v", recovered)
		}
	}()

	_ = config.GetConfig()
	mapper.InitSQL()
	newPassword, newRecoveryKey := service.ResetPasswdForCLI(userName)
	credPath := config.DataPath("reset-credentials.txt")
	content := fmt.Sprintf("用户: %s\n新密码: %s\n恢复密钥: %s\n", strings.TrimSpace(userName), newPassword, newRecoveryKey)
	if writeErr := os.WriteFile(credPath, []byte(content), 0600); writeErr != nil {
		return writeErr
	}
	fmt.Fprintf(stdout, "凭证已写入 %s\n", credPath)
	return nil
}

func run(parent context.Context) error {
	// Create data directories
	if err := os.MkdirAll(filepath.Join(config.DataDir(), "log"), 0755); err != nil {
		return err
	}

	// Load config
	cfg := config.GetConfig()

	// Initialize secure cookie
	middleware.InitSecureCookie()

	// Initialize database
	mapper.InitSQL()
	defer func() {
		if err := mapper.CloseDB(); err != nil {
			log.Printf("Failed to close database: %v", err)
		}
	}()

	// Update abnormal tasks on startup
	mapper.UpdateAbnormalTasks()

	if !service.IsInitialized() {
		config.EnsureSetupToken()
	}

	// Initialize jobs
	service.InitJobs()
	stopTaskRetention := service.StartTaskRetentionScheduler()
	defer stopTaskRetention()

	// gin.Default() installs an access logger that serializes every /assets/*
	// hit to stdout. This binary already recovers panics and the hashed
	// bundles are the hot path, so skip the logger. ReleaseMode also drops
	// gin's debug route dumps when GIN_MODE is unset (local binary runs).
	if os.Getenv("GIN_MODE") == "" {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	if err := r.SetTrustedProxies(nil); err != nil {
		return err
	}

	http3Port := 0
	if cfg.Server.TLSEnabled() {
		http3Port = cfg.Server.Port
	}

	// Error recovery + Auth middleware
	r.Use(errorRecovery())
	r.Use(securityHeaders(http3Port))
	r.Use(maxRequestBodySize(1 << 20)) // 1MB; config/alist/notify payloads are small JSON
	r.Use(maybeCompress())
	r.Use(middleware.CSRFProtection())
	r.Use(middleware.AuthRequired())

	// System routes (no auth needed)
	noAuth := r.Group("/svr/noAuth")
	noAuth.Use(middleware.NoAuthCSRFProtection())
	{
		noAuth.GET("/init", handler.GetInitStatus)
		noAuth.POST("/init", handler.Initialize)
		noAuth.POST("/login", handler.Login)
		noAuth.PUT("/login", handler.ResetPassword)
		noAuth.DELETE("/login", handler.Logout)
	}

	// User routes
	r.GET("/svr/user", handler.GetUser)
	r.PUT("/svr/user", handler.EditPassword)

	// System config routes
	r.GET("/svr/system/config", handler.GetSystemConfig)
	r.PUT("/svr/system/config", handler.UpdateSystemConfig)

	// AList routes
	r.GET("/svr/alist", handler.GetAlist)
	r.POST("/svr/alist", handler.AddAlist)
	r.PUT("/svr/alist", handler.UpdateAlist)
	r.DELETE("/svr/alist", handler.DeleteAlist)

	// Job routes
	r.GET("/svr/job", handler.GetJob)
	r.GET("/svr/job/stream", handler.StreamJobCurrent)
	r.POST("/svr/job", handler.AddJob)
	r.PUT("/svr/job", handler.UpdateJob)
	r.DELETE("/svr/job", handler.DeleteJob)

	// Notify routes
	r.GET("/svr/notify", handler.GetNotify)
	r.POST("/svr/notify", handler.AddNotify)
	r.PUT("/svr/notify", handler.UpdateNotify)
	r.DELETE("/svr/notify", handler.DeleteNotify)

	// Serve frontend static files
	webDist, err := fs.Sub(webFS, "web")
	if err == nil {
		webFiles := newCachedWebFS(webDist)
		webFiles.warm("index.html", "favicon.svg", "icons.svg")
		if assetsDist, err := fs.Sub(webDist, "assets"); err == nil {
			// Vite content-hashes every /assets/* bundle, so an immutable,
			// long-lived Cache-Control is the correct and biggest win: repeat
			// visits never re-download the JS/CSS over the wire.
			assets := r.Group("/assets")
			assets.Use(cachedStaticFiles())
			assets.StaticFS("", http.FS(assetsDist))
		}
		// The SPA shell and icons change on release; always revalidate so the
		// browser still picks up new asset hashes. Body bytes stay in memory
		// because embed.FS is immutable for the process lifetime.
		r.GET("/favicon.svg", func(c *gin.Context) {
			c.Header("Cache-Control", "no-cache")
			webFiles.serve(c, "favicon.svg", "image/svg+xml")
		})
		r.GET("/icons.svg", func(c *gin.Context) {
			c.Header("Cache-Control", "no-cache")
			webFiles.serve(c, "icons.svg", "image/svg+xml")
		})
		r.GET("/", noCacheHTML(), func(c *gin.Context) {
			webFiles.serve(c, "index.html", "text/html; charset=utf-8")
		})
		r.NoRoute(noCacheHTML(), func(c *gin.Context) {
			serveSPAFallbackWithCache(c, webFiles)
		})
	}

	port := fmt.Sprintf("%d", cfg.Server.Port)
	addr := net.JoinHostPort(cfg.Server.Bind, port)
	if cfg.Server.TLSEnabled() {
		log.Printf("启动成功_/_Running at https://%s/ (HTTP/2 + HTTP/3)", addr)
	} else {
		log.Printf("启动成功_/_Running at http://%s/", addr)
	}

	server := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       120 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    32 << 10,
	}
	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return err
	}

	signalCtx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()

	err = runHTTPServer(signalCtx, server, listener, cfg.Server.TLSCertFile, cfg.Server.TLSKeyFile)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	service.ShutdownJobs(shutdownCtx)
	return err
}

func runHTTPServer(ctx context.Context, server *http.Server, listener net.Listener, certFile, keyFile string) error {
	certFile = strings.TrimSpace(certFile)
	keyFile = strings.TrimSpace(keyFile)
	tlsEnabled := certFile != "" && keyFile != ""

	var h3 *http3.Server
	if tlsEnabled {
		tlsConf, err := loadServerTLSConfig(certFile, keyFile)
		if err != nil {
			_ = listener.Close()
			return err
		}
		server.TLSConfig = http12TLSConfig(tlsConf)
		packet, err := net.ListenPacket("udp", listener.Addr().String())
		if err != nil {
			_ = listener.Close()
			return fmt.Errorf("HTTP/3 listen: %w", err)
		}
		h3 = &http3.Server{
			Handler:   server.Handler,
			TLSConfig: http3.ConfigureTLSConfig(tlsConf.Clone()),
		}
		go func() {
			if err := h3.Serve(packet); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Printf("HTTP/3 server: %v", err)
			}
		}()
	}

	errCh := make(chan error, 1)
	go func() {
		if tlsEnabled {
			errCh <- server.ServeTLS(listener, certFile, keyFile)
			return
		}
		errCh <- server.Serve(listener)
	}()

	select {
	case err := <-errCh:
		if h3 != nil {
			_ = h3.Close()
		}
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if h3 != nil {
		_ = h3.Shutdown(shutdownCtx)
	}
	if err := server.Shutdown(shutdownCtx); err != nil {
		_ = server.Close()
		if h3 != nil {
			_ = h3.Close()
		}
		return err
	}

	err := <-errCh
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func loadServerTLSConfig(certFile, keyFile string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("load TLS certificate: %w", err)
	}
	return &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{cert},
	}, nil
}

func http12TLSConfig(base *tls.Config) *tls.Config {
	cfg := base.Clone()
	cfg.NextProtos = []string{"h2", "http/1.1"}
	return cfg
}
