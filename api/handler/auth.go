package handler

import (
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

		// Un pseudo reste réutilisable tant que son détenteur n'est pas connecté.
		if tokenStore.IsTokenValid(loginReq.Username) && middleware.IsUserConnected(loginReq.Username) {
			c.JSON(http.StatusConflict, gin.H{"error": "username already in use"})
			return
		}

		// Generate JWT token
		expiryDuration := time.Duration(env.AccessTokenExpiryHour) * time.Hour
		expiry := time.Now().Add(expiryDuration)
		tokenString, err := middleware.GenerateToken(loginReq.Username, env.AccessTokenSecret, expiryDuration)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not generate token"})
			return
		}

		// Store token in memory
		tokenStore.AddToken(loginReq.Username, expiry)

		c.JSON(http.StatusOK, gin.H{"token": tokenString})
	}
}

// Logout libère le pseudo et retire le joueur du lobby.
func Logout(c *gin.Context) {
	username := c.GetString("username")
	tokenStore.RemoveToken(username)
	if event := removePlayer(username); event != "" {
		broadcastGameUpdate(event, username)
	}
	c.JSON(http.StatusOK, gin.H{"message": "Logged out"})
}

func ListTokens(c *gin.Context) {
	tokens := tokenStore.GetTokens()
	c.JSON(http.StatusOK, gin.H{"tokens": tokens})
}
