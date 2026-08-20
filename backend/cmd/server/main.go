package main

import (
	"context"
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
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
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

func securityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Referrer-Policy", "no-referrer")
		c.Header("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		c.Header("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		c.Next()
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
	serveWebFile(c, webDist, "index.html", "text/html; charset=utf-8")
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
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + port + "/")
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

	r := gin.Default()
	if err := r.SetTrustedProxies(nil); err != nil {
		return err
	}

	// Error recovery + Auth middleware
	r.Use(errorRecovery())
	r.Use(securityHeaders())
	r.Use(maxRequestBodySize(1 << 20)) // 1MB; config/alist/notify payloads are small JSON
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
		if assetsDist, err := fs.Sub(webDist, "assets"); err == nil {
			r.StaticFS("/assets", http.FS(assetsDist))
		}
		r.GET("/favicon.svg", func(c *gin.Context) {
			serveWebFile(c, webDist, "favicon.svg", "image/svg+xml")
		})
		r.GET("/icons.svg", func(c *gin.Context) {
			serveWebFile(c, webDist, "icons.svg", "image/svg+xml")
		})
		r.GET("/", func(c *gin.Context) {
			serveWebFile(c, webDist, "index.html", "text/html; charset=utf-8")
		})
		r.NoRoute(func(c *gin.Context) {
			serveSPAFallback(c, webDist)
		})
	}

	port := fmt.Sprintf("%d", cfg.Server.Port)
	addr := net.JoinHostPort(cfg.Server.Bind, port)
	log.Printf("启动成功_/_Running at http://%s/", addr)

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

	err = runHTTPServer(signalCtx, server, listener)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	service.ShutdownJobs(shutdownCtx)
	return err
}

func runHTTPServer(ctx context.Context, server *http.Server, listener net.Listener) error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Serve(listener)
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		_ = server.Close()
		return err
	}

	err := <-errCh
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
