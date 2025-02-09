package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/becaraya/katana-api/api/middleware"
	"github.com/becaraya/katana-api/internal/bootstrap"
	"github.com/gin-gonic/gin"
)

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
}

var tokenStore = middleware.NewTokenStore()

func Login(env *bootstrap.Env) gin.HandlerFunc {
	return func(c *gin.Context) {
		var loginReq LoginRequest
		if err := c.ShouldBindJSON(&loginReq); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Generate JWT token
		expiry := time.Now().Add(24 * time.Hour)
		tokenString, err := middleware.GenerateToken(loginReq.Username, env.AccessTokenSecret, 24*time.Hour)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not generate token"})
			return
		}

		// Store token in memory
		tokenStore.AddToken(loginReq.Username, expiry)

		// Broadcast new user
		message, _ := json.Marshal(map[string]string{"username": loginReq.Username, "expiry": expiry.String()})
		middleware.BroadcastMessage(message)

		c.JSON(http.StatusOK, gin.H{"token": tokenString})
	}
}

func ListTokens(c *gin.Context) {
	tokens := tokenStore.GetTokens()
	c.JSON(http.StatusOK, gin.H{"tokens": tokens})
}
