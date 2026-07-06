package middleware

import (
	"crynux_bridge/config"
	"net/http"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

func AdminAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.Query("auth")
		expectedToken := config.GetConfig().Admin.AuthToken
		if expectedToken == "" || token != expectedToken {
			log.WithField("path", c.FullPath()).Warn("admin auth failed")
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"message": "unauthorized",
			})
			return
		}

		c.Next()
	}
}
