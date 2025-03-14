package main

import (
	"github.com/Faizan2005/payment-gateway-stripe/config"
	"github.com/Faizan2005/payment-gateway-stripe/models"
	"github.com/Faizan2005/payment-gateway-stripe/routes"
)

func main() {

	db, _ := config.ConnectDB()

	store := models.NewPostgresStorage(db)

	listenAddr := ":3000"
	server := routes.NewAPIServer(listenAddr, store)
	server.Run()
}
