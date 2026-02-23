package main

import "github.com/gin-gonic/gin"

func main() {
	router := gin.Default()

	generateProducts()

	router.GET("/products/search", handleSearch)
	router.GET("/health", handleHealth)

	router.Run(":8080")
}
