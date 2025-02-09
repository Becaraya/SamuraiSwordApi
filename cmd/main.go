package main

import (
	"time"

	route "github.com/becaraya/katana-api/api/route"

	"github.com/becaraya/katana-api/internal/bootstrap"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func main() {

	app := bootstrap.App()

	env := app.Env

	timeout := time.Duration(env.ContextTimeout) * time.Second

	gin := gin.Default()

	gin.Use(cors.New(cors.Config{
		AllowOrigins:     []string{env.FrontendUrl},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	route.Setup(env, timeout, gin)

	gin.Run(env.ServerAddress)
}
