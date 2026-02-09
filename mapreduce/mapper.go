package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"unicode"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/gin-gonic/gin"
)

func main() {
	r := gin.Default()
	r.GET("/map", mapHandler)

	r.Run(":8081")
}

func mapHandler(c *gin.Context) {
	bucket := c.Query("bucket")
	key := c.Query("key")

	if bucket == "" || key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bucket and key required"})
		return
	}

	sess, _ := session.NewSession(&aws.Config{Region: aws.String("us-west-2")})

	svc := s3.New(sess)

	resp, err := svc.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to download file: " + err.Error()})
		return
	}
	defer resp.Body.Close()

	content, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read file content"})
		return
	}

	text := string(content)

	wordCounts := make(map[string]int)

	for _, word := range strings.Fields(text) {
		word = strings.ToLower(word)
		word = strings.TrimFunc(word, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsNumber(r)
		})
		if word != "" {
			wordCounts[word]++
		}
	}

	jsonData, err := json.Marshal(wordCounts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to marshal JSON"})
		return
	}

	outKey := strings.Replace(key, ".txt", "-out.json", 1)

	_, err = svc.PutObject(&s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(outKey),
		Body:   bytes.NewReader(jsonData),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to upload %s: %v", outKey, err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Map successful",
		"result":  outKey,
	})
}
