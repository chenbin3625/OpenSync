package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"taosync/internal/config"
	"taosync/internal/handler"
	"taosync/internal/mapper"
	"taosync/internal/middleware"
	"taosync/internal/model"
	"taosync/internal/service"

	"github.com/gin-gonic/gin"
)

//go:embed all:web
var webFS embed.FS

// errorRecovery catches panics and returns them as 500 responses
func errorRecovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				switch e := r.(type) {
				case string:
					c.JSON(http.StatusOK, model.Error(e))
				case error:
					c.JSON(http.StatusOK, model.Error(e.Error()))
				default:
					c.JSON(http.StatusOK, model.Error(fmt.Sprintf("%v", e)))
				}
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
	// Create data directories
	os.MkdirAll("data/log", 0755)

	// Load config
	cfg := config.GetConfig()

	// Initialize secure cookie
	middleware.InitSecureCookie()

	// Initialize database
	initPasswd := mapper.InitSQL()
	if initPasswd != "" {
		log.Printf("Initial admin password: %s", initPasswd)
	}

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
		noAuth.POST("/login", handler.Login)
		noAuth.PUT("/login", handler.ResetPassword)
		noAuth.DELETE("/login", handler.Logout)
	}

	// User routes
	r.GET("/svr/user", handler.GetUser)
	r.PUT("/svr/user", handler.EditPassword)

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
	if err := r.Run(fmt.Sprintf(":%s", port)); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
