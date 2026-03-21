package main

import (
	"log"

	"github.com/gin-gonic/gin"
)

func main() {
	cfg := LoadConfig()

	cartRepo, err := NewCartRepository(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := cartRepo.Close(); err != nil {
			log.Printf("close repository: %v", err)
		}
	}()

	app := &App{
		products: NewProductStore(),
		carts:    cartRepo,
	}

	router := gin.Default()
	router.GET("/products/:productId", app.handleGetProduct)
	router.POST("/products/:productId/details", app.handlePostProductDetails)
	router.POST("/shopping-carts", app.handlePostShoppingCart)
	router.GET("/shopping-carts/:shoppingCartId", app.handleGetShoppingCart)
	router.POST("/shopping-carts/:shoppingCartId/items", app.handlePostShoppingCartItems)

	if err := router.Run(":" + cfg.Port); err != nil {
		log.Fatal(err)
	}
}
