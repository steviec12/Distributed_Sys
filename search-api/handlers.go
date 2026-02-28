package main

import (
	"bytes"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

var analyticsBreaker = NewCircuitBreaker(5, 30*time.Second)
var analyticsURL = "http://10.0.0.1:9999/analytics"
var analyticsLog [][]byte
var analyticsClient = &http.Client{Timeout: 200 * time.Millisecond}

func logAnalytics(query string) error {
	payload := make([]byte, 512*1024)
	analyticsLog = append(analyticsLog, payload)

	resp, err := analyticsClient.Post(analyticsURL, "application/json", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

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

	if analyticsBreaker.Allow() {
		if err := logAnalytics(query); err != nil {
			analyticsBreaker.RecordFailure()
		} else {
			analyticsBreaker.RecordSuccess()
		}
	}

	c.JSON(http.StatusOK, SearchResponse{
		Products:   results,
		TotalFound: found,
		SearchTime: elapsed,
	})
}

func handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
