package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

type Server struct {
	store       JobStore
	objectStore ObjectStore
}

func main() {
	config := loadConfig()

	client := redis.NewClient(&redis.Options{
		Addr:     config.RedisAddr,
		Password: config.RedisPassword,
		DB:       config.RedisDB,
	})

	redisStore := NewRedisJobStore(client)
	var store JobStore = redisStore
	if config.Dynamo.Enabled {
		durableStore, err := NewDynamoJobStore(context.Background(), config.Dynamo)
		if err != nil {
			log.Fatalf("create dynamodb job store: %v", err)
		}
		store = NewHybridJobStore(redisStore, durableStore)
	}
	objectStore, err := NewObjectStore(context.Background(), config.ObjectStore)
	if err != nil {
		log.Fatalf("create object store: %v", err)
	}
	server := &Server{
		store:       store,
		objectStore: objectStore,
	}

	router := gin.Default()

	router.GET("/health", server.handleHealth)
	router.GET("/metrics", server.handleMetrics)
	router.POST("/uploads", server.handleCreateUpload)
	router.POST("/jobs/:id/finalize", server.handleFinalizeUpload)
	router.GET("/jobs/:id", server.handleGetJob)

	log.Printf("api listening on :%s deployment_mode=%s", config.Port, config.DeploymentMode)
	if err := router.Run(":" + config.Port); err != nil {
		log.Fatalf("run api: %v", err)
	}
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

func (s *Server) handleCreateUpload(c *gin.Context) {
	var request CreateUploadRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid upload request"})
		return
	}
	if strings.TrimSpace(request.FileName) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file_name is required"})
		return
	}
	if request.FileSizeBytes <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file_size_bytes must be greater than 0"})
		return
	}

	jobID, err := newJobID()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create job id"})
		return
	}

	safeFileName := sanitizeFileName(request.FileName)
	contentType := strings.TrimSpace(request.ContentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	session, err := s.objectStore.CreateMultipartUploadSession(c.Request.Context(), jobID, safeFileName, request.FileSizeBytes, contentType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create upload session"})
		return
	}

	job := JobRecord{
		JobID:               jobID,
		Status:              StatusUploading,
		FileName:            safeFileName,
		FileSizeBytes:       request.FileSizeBytes,
		ContentType:         contentType,
		S3Key:               session.S3Key,
		UploadID:            session.UploadID,
		CreatedAt:           utcNow(),
		StartedAt:           "",
		CompletedAt:         "",
		ProcessingStartedAt: "",
		LastHeartbeatAt:     "",
		WorkerID:            "",
		RetryCount:          0,
		FailureType:         FailureTypeNone,
		Error:               "",
		ResultJSON:          "",
	}

	if err := s.store.CreateUploadJob(c.Request.Context(), job); err != nil {
		if abortErr := s.objectStore.AbortMultipartUpload(c.Request.Context(), job.S3Key, job.UploadID); abortErr != nil {
			log.Printf("upload_session_abort_failed job_id=%s upload_id=%s error=%q", job.JobID, job.UploadID, abortErr)
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create upload job"})
		return
	}

	log.Printf("upload_session_created job_id=%s file_name=%s size_bytes=%d s3_key=%s", job.JobID, job.FileName, job.FileSizeBytes, job.S3Key)
	c.JSON(http.StatusAccepted, CreateUploadResponse{
		JobID:    job.JobID,
		Status:   job.Status,
		FileName: job.FileName,
		PartSize: session.PartSize,
		Parts:    session.Parts,
	})
}

func (s *Server) handleFinalizeUpload(c *gin.Context) {
	var request FinalizeUploadRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid finalize request"})
		return
	}
	if len(request.Parts) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "parts are required"})
		return
	}

	job, err := s.store.GetJob(c.Request.Context(), c.Param("id"))
	if err != nil {
		if errors.Is(err, ErrJobNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load job"})
		return
	}
	if job.Status != StatusUploading {
		c.JSON(http.StatusConflict, gin.H{"error": "job is not in uploading state"})
		return
	}
	if job.S3Key == "" || job.UploadID == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "upload session metadata is missing"})
		return
	}

	completedParts := make([]CompletedUploadPart, 0, len(request.Parts))
	for _, part := range request.Parts {
		if part.PartNumber <= 0 || strings.TrimSpace(part.ETag) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "each part requires part_number and etag"})
			return
		}
		completedParts = append(completedParts, CompletedUploadPart{
			PartNumber: part.PartNumber,
			ETag:       part.ETag,
		})
	}

	if err := s.objectStore.CompleteMultipartUpload(c.Request.Context(), job.S3Key, job.UploadID, completedParts); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to complete multipart upload"})
		return
	}

	job.Status = StatusQueued
	job.UploadID = ""
	job.FailureType = FailureTypeNone
	job.Error = ""
	job.ResultJSON = ""

	if err := s.store.FinalizeUploadJob(c.Request.Context(), job); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to finalize upload job"})
		return
	}

	log.Printf("upload_finalized job_id=%s file_name=%s s3_key=%s", job.JobID, job.FileName, job.S3Key)
	c.JSON(http.StatusAccepted, gin.H{
		"job_id":  job.JobID,
		"status":  job.Status,
		"message": "upload finalized successfully",
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

func (s *Server) handleMetrics(c *gin.Context) {
	metrics, err := s.store.GetMetrics(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load metrics"})
		return
	}

	c.JSON(http.StatusOK, metrics)
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

func utcNow() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
}
