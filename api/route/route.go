package route

import (
	"time"

	"github.com/becaraya/katana-api/api/handler"
	"github.com/becaraya/katana-api/api/middleware"
	"github.com/becaraya/katana-api/internal/bootstrap"

	"github.com/gin-gonic/gin"
)

func Setup(env *bootstrap.Env, timeout time.Duration, gin *gin.Engine) {
	publicRouter := gin.Group("")
	{
		publicRouter.POST("/login", handler.Login(env))
		publicRouter.GET("/ws", middleware.WebSocketHandler)

	}

	protectedRouter := gin.Group("")
	protectedRouter.Use(middleware.JWTAuthMiddleware(env.AccessTokenSecret))
	{
		protectedRouter.GET("/tokens", handler.ListTokens)
	}
}
