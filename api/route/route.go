package route

import (
	"time"

	"github.com/becaraya/katana-api/api/handler"
	"github.com/becaraya/katana-api/api/middleware"
	"github.com/becaraya/katana-api/internal/bootstrap"

	"github.com/gin-gonic/gin"
)

func Setup(env *bootstrap.Env, timeout time.Duration, gin *gin.Engine) {
	middleware.SetDisconnectHandler(handler.HandleDisconnect)

	publicRouter := gin.Group("")
	{
		publicRouter.POST("/login", handler.Login(env))
		publicRouter.GET("/ws", middleware.WebSocketHandler)
	}

	protectedRouter := gin.Group("")
	protectedRouter.Use(middleware.JWTAuthMiddleware(env.AccessTokenSecret))
	{
		protectedRouter.GET("/tokens", handler.ListTokens)
		protectedRouter.POST("/logout", handler.Logout)
		protectedRouter.GET("/game", handler.GetGameState)
		protectedRouter.GET("/game/me", handler.GetMyPrivateState)
		protectedRouter.POST("/game/join", handler.JoinGame)
		protectedRouter.POST("/game/leave", handler.LeaveGame)
		protectedRouter.POST("/game/start", handler.StartGame)
		protectedRouter.POST("/game/end", handler.EndGame)
		protectedRouter.POST("/game/ready", handler.SetReady)
		protectedRouter.POST("/game/turn/end", handler.EndTurn)
		protectedRouter.POST("/game/turn/discard", handler.DiscardCards)

		// Routes de test, jamais exposées hors développement
		if env.AppEnv == "development" {
			protectedRouter.POST("/debug/game", handler.DebugCreateGame)
			protectedRouter.DELETE("/debug/game", handler.DebugDeleteGame)
			protectedRouter.POST("/debug/bot", handler.DebugAddBot)
		}
	}
}
