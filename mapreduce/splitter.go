package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/gin-gonic/gin"
)

func main() {
	r := gin.Default()

	r.GET("/split", splitHandler)

	r.Run(":8080")
}

func splitHandler(c *gin.Context) {
	bucket := c.Query("bucket")
	key := c.Query("key")

	if bucket == "" || key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bucket and key parameters are required"})
		return
	}

	//Initialize AWS Session
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String("us-west-2"),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to connect to AWS: " + err.Error()})
		return
	}
	svc := s3.New(sess)

	//Download the file from S3
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
	length := len(text)
	chunkSize := length / 3

	// Split into 3 chunks

	p1End := findNearestSpace(text, chunkSize)
	p2End := findNearestSpace(text, p1End+chunkSize)

	part1 := text[:p1End]
	part2 := text[p1End:p2End]
	part3 := text[p2End:]

	chunks := []string{part1, part2, part3}
	var outputKeys []string

	//Upload chunks back to S3
	for i, chunk := range chunks {
		outKey := fmt.Sprintf("chunk-%d.txt", i+1)
		_, err := svc.PutObject(&s3.PutObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(outKey),
			Body:   bytes.NewReader([]byte(chunk)),
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to upload %s: %v", outKey, err)})
			return
		}
		outputKeys = append(outputKeys, outKey)
	}

	//Return the keys of the chunks
	c.JSON(http.StatusOK, gin.H{
		"message": "Split successful",
		"chunks":  outputKeys,
		"bucket":  bucket,
	})
}

func findNearestSpace(text string, targetIndex int) int {
	if targetIndex >= len(text) {
		return len(text)
	}
	// Search forward for a space
	for i := targetIndex; i < len(text); i++ {
		if text[i] == ' ' || text[i] == '\n' {
			return i
		}
	}
	return len(text)
}
