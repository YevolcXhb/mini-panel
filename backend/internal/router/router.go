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
		apiV1.GET("/websites/nginx/status", website.GetNginxStatus)
		apiV1.POST("/websites/nginx/start", website.StartNginx)
		apiV1.POST("/websites/nginx/stop", website.StopNginx)
		apiV1.POST("/websites/nginx/restart", website.RestartNginx)
		apiV1.POST("/websites/nginx/reload", website.ReloadNginx)
		apiV1.POST("/websites", website.Create)
		apiV1.GET("/websites/:id", website.GetByID)
		apiV1.PUT("/websites/:id", website.Update)
		apiV1.DELETE("/websites/:id", website.Delete)
		apiV1.PUT("/websites/:id/toggle", website.Toggle)

		db := api.NewDatabaseAPI()
		apiV1.GET("/databases", db.List)
		apiV1.POST("/databases", db.Create)
		apiV1.PUT("/databases/:id", db.Update)
		apiV1.DELETE("/databases/:id", db.Delete)
		apiV1.POST("/databases/test", db.TestConnection)
		apiV1.GET("/databases/:id/dbs", db.ListDatabases)
		apiV1.GET("/databases/:id/tables", db.ListTables)
		apiV1.POST("/databases/:id/create-db", db.CreateDatabase)
		apiV1.POST("/databases/:id/create-user", db.CreateUser)

		fw := api.NewFirewallAPI()
		apiV1.GET("/firewall/rules", fw.List)
		apiV1.POST("/firewall/rules", fw.Create)
		apiV1.PUT("/firewall/rules/:id", fw.Update)
		apiV1.DELETE("/firewall/rules/:id", fw.Delete)
		apiV1.POST("/firewall/apply", fw.Apply)
		apiV1.GET("/firewall/status", fw.Status)
		apiV1.POST("/firewall/start", fw.Start)
		apiV1.POST("/firewall/stop", fw.Stop)

		backup := api.NewBackupAPI()
		apiV1.GET("/backups/tasks", backup.ListTasks)
		apiV1.POST("/backups/tasks", backup.CreateTask)
		apiV1.PUT("/backups/tasks/:id", backup.UpdateTask)
		apiV1.DELETE("/backups/tasks/:id", backup.DeleteTask)
		apiV1.POST("/backups/tasks/:id/run", backup.RunBackup)
		apiV1.GET("/backups/records", backup.ListRecords)
		apiV1.DELETE("/backups/records/:id", backup.DeleteRecord)
		apiV1.POST("/backups/records/:id/restore", backup.RestoreBackup)

		agentAPI := api.NewAgentAPI()
		agentAPI.RegisterRoutes(apiV1)

		sysAPI := api.NewSystemAPI()
		apiV1.GET("/system/services", sysAPI.CheckServices)
		apiV1.POST("/system/services/:name/install", sysAPI.InstallService)
		apiV1.POST("/system/services/:name/start", sysAPI.StartService)
		apiV1.POST("/system/services/:name/stop", sysAPI.StopService)
		apiV1.POST("/system/services/:name/restart", sysAPI.RestartService)

		user := api.NewUserAPI()
		adminV1 := apiV1.Group("/")
		adminV1.Use(middleware.AdminMiddleware())
		{
			adminV1.GET("/users", user.List)
			adminV1.POST("/users", user.Create)
			adminV1.PUT("/users/:id", user.Update)
			adminV1.POST("/users/:id/reset-password", user.ResetPassword)
			adminV1.DELETE("/users/:id", user.Delete)
		}
	}

	return r
}
