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

func serveWebFile(c *gin.Context, webDist fs.FS, filePath, contentType string) {
	data, err := fs.ReadFile(webDist, filePath)
	if err != nil {
		c.String(http.StatusNotFound, "Frontend not found")
		return
	}
	c.Data(http.StatusOK, contentType, data)
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
	// Create data directories
	if err := os.MkdirAll("data/log", 0755); err != nil {
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

	// Initialize jobs
	service.InitJobs()

	r := gin.Default()

	// Error recovery + Auth middleware
	r.Use(errorRecovery())
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

	// Language routes
	r.GET("/svr/language", handler.GetLanguage)
	r.POST("/svr/language", handler.SetLanguage)

	// AList routes
	r.GET("/svr/alist", handler.GetAlist)
	r.POST("/svr/alist", handler.AddAlist)
	r.PUT("/svr/alist", handler.UpdateAlist)
	r.DELETE("/svr/alist", handler.DeleteAlist)

	// Job routes
	r.GET("/svr/job", handler.GetJob)
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
	}

	port := fmt.Sprintf("%d", cfg.Server.Port)
	log.Printf("启动成功_/_Running at http://127.0.0.1:%s/", port)

	server := &http.Server{
		Addr:    fmt.Sprintf(":%s", port),
		Handler: r,
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
