package main

import (
	"github.com/becaraya/katana-api/bootstrap"
	"github.com/gin-gonic/gin"
)

func main() {

	app := bootstrap.App()

	env := app.Env

	// timeout := time.Duration(env.ContextTimeout) * time.Second

	gin := gin.Default()

	gin.Run(env.ServerAddress)
}
