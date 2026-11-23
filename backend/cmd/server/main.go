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
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
)

//go:embed all:web
var webFS embed.FS

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

func securityHeaders(http3Ports ...int) gin.HandlerFunc {
	altSvc := ""
	if len(http3Ports) > 0 && http3Ports[0] > 0 {
		altSvc = fmt.Sprintf(`h3=":%d"; ma=86400`, http3Ports[0])
	}
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Referrer-Policy", "no-referrer")
		if altSvc != "" {
			c.Header("Alt-Svc", altSvc)
			c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		c.Next()
	}
}

func responseWriteDeadline(timeout time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.URL.Path != "/svr/job/stream" {
			_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(timeout))
		}
		c.Next()
	}
}

// assetWriter adds cache headers to successful asset responses and, when gz is
// set, compresses the body. Assets served from embed.FS have no validators, so
// the remaining headers are set by the file server itself.
type assetWriter struct {
	gin.ResponseWriter
	gz       *gzip.Writer
	disabled bool
}

func (w *assetWriter) WriteHeader(code int) {
	// Only successful responses are immutable; a missing asset must not be
	// cached for a year just because its name looks hashed.
	if code == http.StatusOK {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	if w.gz == nil {
		w.ResponseWriter.WriteHeader(code)
		return
	}
	if code == http.StatusNotModified || code == http.StatusNoContent {
		w.disabled = true
		w.Header().Del("Content-Encoding")
		w.Header().Del("Content-Length")
		w.ResponseWriter.WriteHeader(code)
		return
	}
	w.Header().Set("Content-Encoding", "gzip")
	// ServeContent sets the uncompressed length; it is invalid once compressed
	// and must be dropped so the response uses chunked transfer encoding.
	w.Header().Del("Content-Length")
	w.ResponseWriter.WriteHeader(code)
}

func (w *assetWriter) Write(data []byte) (int, error) {
	if w.gz == nil || w.disabled {
		return w.ResponseWriter.Write(data)
	}
	return w.gz.Write(data)
}

func acceptsGzip(r *http.Request) bool {
	for _, part := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
		encoding := strings.TrimSpace(strings.SplitN(part, ";", 2)[0])
		if strings.EqualFold(encoding, "gzip") {
			return true
		}
	}
	return false
}

// serveAssetFile serves Vite's content-hashed bundle with a long-lived cache
// header and, when the client accepts it, gzip compression. Range requests and
// HEAD are passed through uncompressed because ranges would address the
// uncompressed representation.
func serveAssetFile(fileServer http.Handler) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Vary", "Accept-Encoding")
		request := c.Request
		writer := &assetWriter{ResponseWriter: c.Writer}
		if request.Method != http.MethodHead && request.Header.Get("Range") == "" && acceptsGzip(request) {
			writer.gz = gzip.NewWriter(c.Writer)
		}
		fileServer.ServeHTTP(writer, request)
		if writer.gz != nil && !writer.disabled {
			_ = writer.gz.Close()
		}
	}
}

func serveWebFile(c *gin.Context, webDist fs.FS, filePath, contentType string) {
	data, err := fs.ReadFile(webDist, filePath)
	if err != nil {
		c.String(http.StatusNotFound, "Frontend not found")
		return
	}
	c.Data(http.StatusOK, contentType, data)
}

func serveSPAFallback(c *gin.Context, webDist fs.FS) {
	requestPath := c.Request.URL.Path
	if c.Request.Method != http.MethodGet ||
		requestPath == "/svr" ||
		strings.HasPrefix(requestPath, "/svr/") ||
		strings.HasPrefix(requestPath, "/assets/") ||
		pathpkg.Ext(requestPath) != "" {
		c.Status(http.StatusNotFound)
		return
	}
	c.Header("Cache-Control", "no-cache")
	serveWebFile(c, webDist, "index.html", "text/html; charset=utf-8")
}

func configureTrustedProxies(r *gin.Engine, proxies []string) error {
	if len(proxies) == 0 {
		return r.SetTrustedProxies(nil)
	}
	return r.SetTrustedProxies(proxies)
}

func validateTLSFiles(certFile, keyFile string) error {
	hasCert := strings.TrimSpace(certFile) != ""
	hasKey := strings.TrimSpace(keyFile) != ""
	if hasCert != hasKey {
		return errors.New("incomplete TLS configuration: both OPENSYNC_TLS_CERT and OPENSYNC_TLS_KEY must be set")
	}
	return nil
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
	default:
		return false, nil
	}
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
	fmt.Fprintf(stdout, "用户: %s\n新密码: %s\n恢复密钥: %s\n", strings.TrimSpace(userName), newPassword, newRecoveryKey)
	return nil
}

func run(parent context.Context) error {
	// Load config before creating runtime directories so OPENSYNC_DATA_DIR is
	// applied consistently to the database, secret, config, and logs.
	cfg := config.GetConfig()
	if err := validateTLSFiles(cfg.Server.TLSCertFile, cfg.Server.TLSKeyFile); err != nil {
		return err
	}
	if !cfg.Server.TLSEnabled() && !cfg.Server.AllowInsecureHTTP && !isLoopbackBind(cfg.Server.Bind) {
		return errors.New("refusing insecure non-loopback HTTP; configure TLS or OPENSYNC_ALLOW_INSECURE_HTTP=true")
	}

	// Create data directories
	if err := os.MkdirAll(config.DataPath("log"), 0755); err != nil {
		return err
	}

	// Initialize secure cookie
	middleware.InitSecureCookie()

	// Initialize database
	mapper.InitSQL()
	defer func() {
		if err := mapper.CloseDB(); err != nil {
			log.Printf("Failed to close database: %v", err)
		}
	}()

	// Initialize jobs (also marks leftover in-flight tasks as aborted on startup)
	service.InitJobs()
	stopTaskRetention := service.StartTaskRetentionScheduler()
	defer stopTaskRetention()

	r := gin.Default()
	if err := configureTrustedProxies(r, cfg.Server.TrustedProxies); err != nil {
		return err
	}

	// Error recovery + Auth middleware
	r.Use(errorRecovery())
	http3Port := 0
	if cfg.Server.TLSEnabled() {
		http3Port = cfg.Server.Port
	}
	r.Use(securityHeaders(http3Port))
	r.Use(responseWriteDeadline(30 * time.Second))
	r.Use(maxRequestBodySize(1 << 20)) // 1MB; config/alist/notify payloads are small JSON
	r.Use(middleware.AuthRequired())

	// System routes (no auth needed)
	noAuth := r.Group("/svr/noAuth")
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
	r.POST("/svr/notify/test", handler.TestNotify)
	r.PUT("/svr/notify", handler.UpdateNotify)
	r.DELETE("/svr/notify", handler.DeleteNotify)

	// Serve frontend static files
	webDist, err := fs.Sub(webFS, "web")
	if err == nil {
		if assetsDist, err := fs.Sub(webDist, "assets"); err == nil {
			assetHandler := serveAssetFile(http.StripPrefix("/assets", http.FileServer(http.FS(assetsDist))))
			r.GET("/assets/*filepath", assetHandler)
			r.HEAD("/assets/*filepath", assetHandler)
		}
		r.GET("/favicon.svg", func(c *gin.Context) {
			c.Header("Cache-Control", "public, max-age=86400")
			serveWebFile(c, webDist, "favicon.svg", "image/svg+xml")
		})
		r.GET("/icons.svg", func(c *gin.Context) {
			c.Header("Cache-Control", "public, max-age=86400")
			serveWebFile(c, webDist, "icons.svg", "image/svg+xml")
		})
		r.GET("/", func(c *gin.Context) {
			// index.html references hashed assets, so it must be revalidated on
			// every load; the assets themselves are cached immutably above.
			c.Header("Cache-Control", "no-cache")
			serveWebFile(c, webDist, "index.html", "text/html; charset=utf-8")
		})
		r.NoRoute(func(c *gin.Context) {
			serveSPAFallback(c, webDist)
		})
	}

	port := fmt.Sprintf("%d", cfg.Server.Port)
	addr := net.JoinHostPort(cfg.Server.Bind, port)
	if cfg.Server.TLSEnabled() {
		log.Printf("启动成功_/_Running at https://%s/ (HTTP/2 + HTTP/3)", addr)
	} else {
		log.Printf("启动成功_/_Running at http://%s/", addr)
	}

	serverBaseCtx, cancelServerBase := context.WithCancel(context.Background())
	defer cancelServerBase()

	server := &http.Server{
		Addr:              addr,
		Handler:           r,
		BaseContext:       func(net.Listener) context.Context { return serverBaseCtx },
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
	stopCancelBase := context.AfterFunc(signalCtx, cancelServerBase)
	defer stopCancelBase()

	err = runHTTPServer(signalCtx, server, listener, cfg.Server.TLSCertFile, cfg.Server.TLSKeyFile)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	service.ShutdownJobs(shutdownCtx)
	return err
}

func isLoopbackBind(bind string) bool {
	bind = strings.TrimSpace(bind)
	if bind == "localhost" {
		return true
	}
	ip := net.ParseIP(bind)
	return ip != nil && ip.IsLoopback()
}

func runHTTPServer(ctx context.Context, server *http.Server, listener net.Listener, tlsFiles ...string) error {
	certFile, keyFile := "", ""
	if len(tlsFiles) > 0 {
		certFile = strings.TrimSpace(tlsFiles[0])
	}
	if len(tlsFiles) > 1 {
		keyFile = strings.TrimSpace(tlsFiles[1])
	}
	if err := validateTLSFiles(certFile, keyFile); err != nil {
		_ = listener.Close()
		return err
	}
	tlsEnabled := certFile != "" && keyFile != ""

	serverBaseCtx, cancelServerBase := context.WithCancel(context.Background())
	defer cancelServerBase()
	if server.BaseContext == nil {
		server.BaseContext = func(net.Listener) context.Context { return serverBaseCtx }
	} else {
		prevBase := server.BaseContext
		server.BaseContext = func(ln net.Listener) context.Context {
			base := prevBase(ln)
			c, cancel := context.WithCancel(base)
			stop := context.AfterFunc(serverBaseCtx, cancel)
			context.AfterFunc(c, func() { stop() })
			return c
		}
	}

	var h3 *http3.Server
	var h3Packet net.PacketConn
	var h3Done chan error
	shutdownH3 := func(shutdownCtx context.Context) error {
		if h3 == nil {
			return nil
		}
		err := h3.Shutdown(shutdownCtx)
		if err != nil {
			_ = h3.Close()
		}
		if h3Done != nil {
			select {
			case serveErr := <-h3Done:
				h3Done = nil
				if err == nil && serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) && !errors.Is(serveErr, net.ErrClosed) {
					err = serveErr
				}
			case <-shutdownCtx.Done():
				_ = h3.Close()
				log.Printf("HTTP/3 server shutdown timed out")
				if err == nil {
					err = shutdownCtx.Err()
				}
			}
		}
		if h3Packet != nil {
			_ = h3Packet.Close()
			h3Packet = nil
		}
		return err
	}
	defer func() {
		if h3 != nil && h3Done != nil {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			_ = shutdownH3(cleanupCtx)
			cancel()
		} else if h3Packet != nil {
			_ = h3Packet.Close()
			h3Packet = nil
		}
	}()

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
		h3Packet = packet
		h3 = &http3.Server{
			Handler:        server.Handler,
			TLSConfig:      http3.ConfigureTLSConfig(tlsConf.Clone()),
			MaxHeaderBytes: server.MaxHeaderBytes,
			IdleTimeout:    server.IdleTimeout,
			ConnContext: func(connCtx context.Context, _ *quic.Conn) context.Context {
				c, cancel := context.WithCancel(connCtx)
				stop := context.AfterFunc(serverBaseCtx, cancel)
				context.AfterFunc(c, func() { stop() })
				return c
			},
		}
		h3Done = make(chan error, 1)
		go func() {
			h3Done <- h3.Serve(packet)
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
		cancelServerBase()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		h3Err := shutdownH3(shutdownCtx)
		cancel()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		if h3Err != nil && !errors.Is(h3Err, http.ErrServerClosed) && !errors.Is(h3Err, net.ErrClosed) {
			return fmt.Errorf("HTTP/3 server error: %w", h3Err)
		}
		return nil
	case err := <-h3Done:
		h3Done = nil
		cancelServerBase()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = server.Shutdown(shutdownCtx)
		cancel()
		_ = server.Close()
		<-errCh
		if h3Packet != nil {
			_ = h3Packet.Close()
			h3Packet = nil
		}
		if err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
			return fmt.Errorf("HTTP/3 server error: %w", err)
		}
		return nil
	case <-ctx.Done():
	}

	cancelServerBase()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	h3Err := shutdownH3(shutdownCtx)
	srvErr := server.Shutdown(shutdownCtx)
	if srvErr != nil {
		_ = server.Close()
	}
	serveErr := <-errCh
	if srvErr != nil {
		return srvErr
	}
	if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		return serveErr
	}
	if h3Err != nil && !errors.Is(h3Err, http.ErrServerClosed) && !errors.Is(h3Err, net.ErrClosed) {
		return fmt.Errorf("HTTP/3 shutdown: %w", h3Err)
	}
	return nil
}

func loadServerTLSConfig(certFile, keyFile string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("load TLS certificate: %w", err)
	}
	return &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{cert}}, nil
}

func http12TLSConfig(base *tls.Config) *tls.Config {
	cfg := base.Clone()
	cfg.NextProtos = []string{"h2", "http/1.1"}
	return cfg
}
