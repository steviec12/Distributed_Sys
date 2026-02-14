package main

import "github.com/gin-gonic/gin"

func main() {
	router := gin.Default()

	// Register routes — match the api.yaml paths
	router.GET("/products/:productId", handleGetProduct)
	router.POST("/products/:productId/details", handlePostProductDetails)

	router.Run(":8080")
}
