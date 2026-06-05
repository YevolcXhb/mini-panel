package router

import (
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
	r.Use(middleware.SecurityEntranceMiddleware())

	if _, err := os.Stat("static/index.html"); err == nil {
		r.Static("/assets", "static/assets")
		r.StaticFile("/favicon.ico", "static/favicon.ico")
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

		ver := api.NewVersionAPI()
		authGroup.GET("/version", ver.Get)
	}

	apiV1 := r.Group("/api/v1")
	apiV1.Use(middleware.AuthMiddleware())
	{
		dash := api.NewDashboardAPI()
		apiV1.GET("/dashboard", dash.GetInfo)
		apiV1.GET("/dashboard/monitor", dash.GetMonitor)

		file := api.NewFileAPI()
		apiV1.GET("/files", file.List)
		apiV1.GET("/files/content", file.GetContent)
		apiV1.POST("/files", file.Create)
		apiV1.PUT("/files", file.Update)
		apiV1.DELETE("/files", file.Delete)
		apiV1.POST("/files/upload", file.Upload)
		apiV1.GET("/files/download", file.Download)

		term := api.NewTerminalAPI()
		apiV1.GET("/terminal/ws", term.HandleWS)

		proc := api.NewProcessAPI()
		apiV1.GET("/processes", proc.List)
		apiV1.POST("/processes/kill", proc.Kill)

		ctn := api.NewContainerAPI()
		apiV1.GET("/containers", ctn.List)
		apiV1.GET("/containers/:name", ctn.Inspect)
		apiV1.POST("/containers", ctn.Create)
		apiV1.POST("/containers/:name/start", ctn.Start)
		apiV1.POST("/containers/:name/stop", ctn.Stop)
		apiV1.DELETE("/containers/:name", ctn.Remove)
		apiV1.GET("/containers/:name/logs", ctn.Logs)
		apiV1.GET("/containers/:name/files", ctn.ListFiles)
		apiV1.POST("/containers/pull", ctn.Pull)

		app := api.NewAppAPI()
		apiV1.GET("/apps", app.List)
		apiV1.GET("/apps/search", app.Search)
		apiV1.GET("/apps/icon/:key", app.Icon)
		apiV1.GET("/apps/:id", app.Detail)
		apiV1.POST("/apps/install", app.Install)
		apiV1.POST("/apps/:id/uninstall", app.Uninstall)
		apiV1.GET("/apps/installed", app.Installed)
		apiV1.POST("/apps/sync", app.Sync)
		apiV1.GET("/apps/sources", app.Sources)
		apiV1.POST("/apps/sources", app.AddSource)
		apiV1.DELETE("/apps/sources/:id", app.RemoveSource)

		cron := api.NewCronjobAPI()
		apiV1.GET("/cronjobs", cron.List)
		apiV1.POST("/cronjobs", cron.Create)
		apiV1.PUT("/cronjobs/:id", cron.Update)
		apiV1.DELETE("/cronjobs/:id", cron.Delete)
		apiV1.POST("/cronjobs/:id/run", cron.Run)

		setting := api.NewSettingAPI()
		apiV1.GET("/settings", setting.Get)
		apiV1.PUT("/settings", setting.Update)
		apiV1.POST("/settings/reset", setting.Reset)
		apiV1.POST("/settings/clear-data", setting.ClearData)

		monitor := api.NewMonitorAPI()
		apiV1.GET("/monitor/history", monitor.List)

		apiV1.POST("/auth/change-password", auth.ChangePassword)

		audit := api.NewAuditAPI()
		apiV1.GET("/audit-logs", audit.List)

		logAPI := api.NewLogAPI()
		apiV1.GET("/logs", logAPI.List)

		website := api.NewWebsiteAPI()
		apiV1.GET("/websites", website.List)
		apiV1.POST("/websites", website.Create)
		apiV1.GET("/websites/:id", website.GetByID)
		apiV1.PUT("/websites/:id", website.Update)
		apiV1.DELETE("/websites/:id", website.Delete)
		apiV1.PUT("/websites/:id/toggle", website.Toggle)
		apiV1.POST("/websites/reload-nginx", website.ReloadNginx)
	}

	return r
}
