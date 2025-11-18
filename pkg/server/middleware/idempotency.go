package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// IdempotencyMiddleware rejects duplicate modification requests using an in-memory token bucket.
type IdempotencyMiddleware struct {
	ttl    time.Duration
	tokens sync.Map
}

func NewIdempotencyMiddleware(ttl time.Duration) *IdempotencyMiddleware {
	return &IdempotencyMiddleware{ttl: ttl}
}

func (m *IdempotencyMiddleware) Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodGet {
			return
		}
		token := c.GetHeader("X-Idempotency-Token")
		if token == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "missing X-Idempotency-Token header"})
			return
		}
		if expiresRaw, ok := m.tokens.Load(token); ok {
			if expiresAt, valid := expiresRaw.(time.Time); valid && time.Now().Before(expiresAt) {
				c.AbortWithStatusJSON(http.StatusConflict, gin.H{"error": "duplicate idempotency token"})
				return
			}
			m.tokens.Delete(token)
		}
		m.tokens.Store(token, time.Now().Add(m.ttl))
	}
}
