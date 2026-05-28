package router

import (
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/minipanel/minipanel/internal/api"
	"github.com/minipanel/minipanel/internal/middleware"
)

func NewRouter() *gin.Engine {
	r := gin.New()
	r.Use(middleware.LoggerMiddleware())
	r.Use(gin.Recovery())
	r.Use(middleware.CORSMiddleware())

	// Static files
	if _, err := os.Stat("static/index.html"); err == nil {
		r.StaticFS("/static", http.Dir("static"))
		r.StaticFile("/", "static/index.html")
		r.NoRoute(func(c *gin.Context) {
			c.File("static/index.html")
		})
	}

	auth := api.NewAuthAPI()
	authGroup := r.Group("/api/v1")
	{
		authGroup.POST("/login", auth.Login)
		authGroup.POST("/logout", auth.Logout)
	}

	apiV1 := r.Group("/api/v1")
	apiV1.Use(middleware.AuthMiddleware())
	{
		// Dashboard
		dash := api.NewDashboardAPI()
		apiV1.GET("/dashboard", dash.GetInfo)
		apiV1.GET("/dashboard/monitor", dash.GetMonitor)

		// File Manager
		file := api.NewFileAPI()
		apiV1.GET("/files", file.List)
		apiV1.GET("/files/content", file.GetContent)
		apiV1.POST("/files", file.Create)
		apiV1.PUT("/files", file.Update)
		apiV1.DELETE("/files", file.Delete)
		apiV1.POST("/files/upload", file.Upload)
		apiV1.GET("/files/download", file.Download)

		// Terminal
		term := api.NewTerminalAPI()
		apiV1.GET("/terminal/ws", term.HandleWS)

		// Process
		proc := api.NewProcessAPI()
		apiV1.GET("/processes", proc.List)
		apiV1.POST("/processes/kill", proc.Kill)

		// Container
		ctn := api.NewContainerAPI()
		apiV1.GET("/containers", ctn.List)
		apiV1.GET("/containers/:name", ctn.Inspect)
		apiV1.POST("/containers", ctn.Create)
		apiV1.POST("/containers/:name/start", ctn.Start)
		apiV1.POST("/containers/:name/stop", ctn.Stop)
		apiV1.DELETE("/containers/:name", ctn.Remove)
		apiV1.GET("/containers/:name/logs", ctn.Logs)
		apiV1.GET("/containers/:name/files", ctn.ListFiles)

		// App Store
		app := api.NewAppAPI()
		apiV1.GET("/apps", app.List)
		apiV1.POST("/apps/install", app.Install)
		apiV1.POST("/apps/:id/uninstall", app.Uninstall)
		apiV1.GET("/apps/installed", app.Installed)

		// Cronjob
		cron := api.NewCronjobAPI()
		apiV1.GET("/cronjobs", cron.List)
		apiV1.POST("/cronjobs", cron.Create)
		apiV1.PUT("/cronjobs/:id", cron.Update)
		apiV1.DELETE("/cronjobs/:id", cron.Delete)
		apiV1.POST("/cronjobs/:id/run", cron.Run)

		// Settings
		setting := api.NewSettingAPI()
		apiV1.GET("/settings", setting.Get)
		apiV1.PUT("/settings", setting.Update)
		apiV1.POST("/settings/reset", setting.Reset)
	}

	return r
}
