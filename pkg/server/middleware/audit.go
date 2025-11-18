package middleware

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/chaosblade-io/chaosblade-spec-go/log"
)

// AuditMiddleware records request lifecycle for traceability.
func AuditMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		duration := time.Since(start)
		log.Infof(c.Request.Context(), "audit: method=%s path=%s status=%d duration=%s", c.Request.Method, c.Request.URL.Path, c.Writer.Status(), duration)
	}
}
