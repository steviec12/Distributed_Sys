package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

type Config struct {
	Port          string
	RedisAddr     string
	RedisPassword string
	RedisDB       int
	UploadDir     string
}

type Server struct {
	store     *JobStore
	uploadDir string
}

func main() {
	config := loadConfig()

	if err := os.MkdirAll(config.UploadDir, 0o755); err != nil {
		log.Fatalf("create upload directory: %v", err)
	}

	client := redis.NewClient(&redis.Options{
		Addr:     config.RedisAddr,
		Password: config.RedisPassword,
		DB:       config.RedisDB,
	})

	store := NewJobStore(client)
	server := &Server{
		store:     store,
		uploadDir: config.UploadDir,
	}

	router := gin.Default()
	router.MaxMultipartMemory = 256 << 20

	router.GET("/health", server.handleHealth)
	router.POST("/jobs", server.handleCreateJob)
	router.GET("/jobs/:id", server.handleGetJob)

	log.Printf("api listening on :%s", config.Port)
	if err := router.Run(":" + config.Port); err != nil {
		log.Fatalf("run api: %v", err)
	}
}

func loadConfig() Config {
	redisDB, err := strconv.Atoi(getEnv("REDIS_DB", "0"))
	if err != nil {
		log.Fatalf("parse REDIS_DB: %v", err)
	}

	return Config{
		Port:          getEnv("PORT", "8080"),
		RedisAddr:     getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword: getEnv("REDIS_PASSWORD", ""),
		RedisDB:       redisDB,
		UploadDir:     getEnv("UPLOAD_DIR", "./uploads"),
	}
}

func getEnv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func (s *Server) handleHealth(c *gin.Context) {
	if err := s.store.Ping(c.Request.Context()); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "unhealthy",
			"error":  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (s *Server) handleCreateJob(c *gin.Context) {
	fileHeader, err := c.FormFile("video")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "video file is required"})
		return
	}

	jobID, err := newJobID()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create job id"})
		return
	}

	safeFileName := sanitizeFileName(fileHeader.Filename)
	storedFileName := fmt.Sprintf("%s-%s", jobID, safeFileName)
	savedPath := filepath.Join(s.uploadDir, storedFileName)
	if err := c.SaveUploadedFile(fileHeader, savedPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save uploaded video"})
		return
	}

	job := JobRecord{
		JobID:         jobID,
		Status:        StatusQueued,
		FilePath:      savedPath,
		FileName:      safeFileName,
		FileSizeBytes: fileHeader.Size,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		StartedAt:     "",
		CompletedAt:   "",
		Error:         "",
		ResultJSON:    "",
	}

	if err := s.store.CreateJob(c.Request.Context(), job); err != nil {
		_ = os.Remove(savedPath)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to submit job"})
		return
	}

	log.Printf("job_submitted job_id=%s file_name=%s size_bytes=%d", job.JobID, job.FileName, job.FileSizeBytes)
	c.JSON(http.StatusAccepted, gin.H{
		"message": "job submitted successfully",
		"job_id":  job.JobID,
		"status":  job.Status,
	})
}

func (s *Server) handleGetJob(c *gin.Context) {
	job, err := s.store.GetJob(c.Request.Context(), c.Param("id"))
	if err != nil {
		if errors.Is(err, ErrJobNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load job"})
		return
	}

	response, err := buildJobResponse(job)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to decode job result"})
		return
	}

	c.JSON(http.StatusOK, response)
}

func newJobID() (string, error) {
	bytes := make([]byte, 12)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func sanitizeFileName(name string) string {
	base := filepath.Base(name)
	base = strings.ReplaceAll(base, " ", "_")
	base = strings.TrimSpace(base)
	if base == "." || base == "" {
		return "upload.bin"
	}
	return base
}
