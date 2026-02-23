package main

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// handleSearch handles GET /products/search?q={query}
func handleSearch(c *gin.Context) {
	query := c.Query("q")

	if query == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "INVALID_INPUT",
			Message: "query parameter 'q' is required",
		})
		return
	}

	start := time.Now()
	results, found := searchProducts(query)
	elapsed := time.Since(start).String()

	c.JSON(http.StatusOK, SearchResponse{
		Products:   results,
		TotalFound: found,
		SearchTime: elapsed,
	})
}

// handleHealth handles GET /health for ALB health checks.
func handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
