package middlewares

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/hris-management-system/go-kit/logger"
)

func GinLogging() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		c.Next()

		requestID := uuid.NewString()
		latency := time.Since(start)
		status := c.Writer.Status()
		clientIP := c.ClientIP()
		method := c.Request.Method

		if raw != "" {
			path = path + "?" + raw
		}

		msg := fmt.Sprintf("%s | %d | %v | %s | %-7s %s",
			requestID, status, latency, clientIP, method, path)

		switch {
		case status >= 500:
			logger.Sugar.Error(msg)
		case status >= 400:
			logger.Sugar.Info(msg)
		default:
			logger.Sugar.Info(msg)
		}
	}
}
