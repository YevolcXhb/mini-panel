package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/minipanel/minipanel/internal/global"
)

func LoggerMiddleware() gin.HandlerFunc {
	return gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		global.LOG.Infof("[%s] %s %s %d %s %s",
			param.TimeStamp.Format(time.RFC3339),
			param.Method,
			param.Path,
			param.StatusCode,
			param.Latency,
			param.ClientIP,
		)
		return ""
	})
}
