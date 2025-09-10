package middlewares

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func LimitBodySize(limitBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limitBytes)
		c.Next()
	}
}
