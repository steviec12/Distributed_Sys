package main

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/gin-gonic/gin"
)

func main() {
	r := gin.Default()

	// Endpoint: /reduce?bucket=NAME&keys=FILE1,FILE2,FILE3
	r.GET("/reduce", reduceHandler)

	r.Run(":8082")
}

func reduceHandler(c *gin.Context) {
	bucket := c.Query("bucket")
	keysParam := c.Query("keys")

	if bucket == "" || keysParam == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bucket and keys parameters are required"})
		return
	}

	keys := strings.Split(keysParam, ",")
	finalCounts := make(map[string]int)

	// 1. Initialize AWS Session
	sess, _ := session.NewSession(&aws.Config{Region: aws.String("us-west-2")})
	svc := s3.New(sess)

	// 2. Loop through each partial result file
	for _, key := range keys {
		// Download file
		resp, err := svc.GetObject(&s3.GetObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(key),
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to download " + key + ": " + err.Error()})
			return
		}
		defer resp.Body.Close()

		content, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read content of " + key})
			return
		}

		// Parse JSON
		var partialCounts map[string]int
		if err := json.Unmarshal(content, &partialCounts); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse JSON for " + key})
			return
		}

		// 3. Aggregate counts
		for word, count := range partialCounts {
			finalCounts[word] += count
		}
	}

	// 4. Save Final Result
	jsonData, err := json.MarshalIndent(finalCounts, "", "  ") // Indent for readability
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to marshal final JSON"})
		return
	}

	outKey := "final-result.json"
	_, err = svc.PutObject(&s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(outKey),
		Body:   bytes.NewReader(jsonData),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to upload final result"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Reduce successful",
		"result":  outKey,
	})
}
