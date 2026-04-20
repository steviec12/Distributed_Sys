package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

type fakeJobStore struct {
	createUploadErr   error
	finalizeUploadErr error
	getJob            JobRecord
	getErr            error
	pingErr           error
	metrics           MetricsResponse
	metricsErr        error

	createdUploadJobs   []JobRecord
	finalizedUploadJobs []JobRecord
}

func (f *fakeJobStore) CreateUploadJob(_ context.Context, job JobRecord) error {
	f.createdUploadJobs = append(f.createdUploadJobs, job)
	return f.createUploadErr
}

func (f *fakeJobStore) FinalizeUploadJob(_ context.Context, job JobRecord) error {
	f.finalizedUploadJobs = append(f.finalizedUploadJobs, job)
	return f.finalizeUploadErr
}

func (f *fakeJobStore) GetJob(_ context.Context, _ string) (JobRecord, error) {
	return f.getJob, f.getErr
}

func (f *fakeJobStore) Ping(_ context.Context) error {
	return f.pingErr
}

func (f *fakeJobStore) GetMetrics(_ context.Context) (MetricsResponse, error) {
	return f.metrics, f.metricsErr
}

type fakeObjectStore struct {
	session         MultipartUploadSession
	sessionErr      error
	completeErr     error
	abortErr        error
	lastCreateJobID string
	lastCreateName  string
	lastCreateSize  int64
	lastCreateType  string
	completedKey    string
	completedUpload string
	completedParts  []CompletedUploadPart
	abortedKey      string
	abortedUpload   string
}

func (f *fakeObjectStore) CreateMultipartUploadSession(_ context.Context, jobID, fileName string, fileSizeBytes int64, contentType string) (MultipartUploadSession, error) {
	f.lastCreateJobID = jobID
	f.lastCreateName = fileName
	f.lastCreateSize = fileSizeBytes
	f.lastCreateType = contentType
	return f.session, f.sessionErr
}

func (f *fakeObjectStore) CompleteMultipartUpload(_ context.Context, s3Key, uploadID string, parts []CompletedUploadPart) error {
	f.completedKey = s3Key
	f.completedUpload = uploadID
	f.completedParts = append([]CompletedUploadPart(nil), parts...)
	return f.completeErr
}

func (f *fakeObjectStore) AbortMultipartUpload(_ context.Context, s3Key, uploadID string) error {
	f.abortedKey = s3Key
	f.abortedUpload = uploadID
	return f.abortErr
}

func newTestRouter(store JobStore, objectStore ObjectStore) *gin.Engine {
	gin.SetMode(gin.TestMode)
	server := &Server{
		store:       store,
		objectStore: objectStore,
	}

	router := gin.New()
	router.POST("/uploads", server.handleCreateUpload)
	router.POST("/jobs/:id/finalize", server.handleFinalizeUpload)
	router.GET("/jobs/:id", server.handleGetJob)
	router.GET("/health", server.handleHealth)
	router.GET("/metrics", server.handleMetrics)
	return router
}

func TestHandleCreateUploadSuccess(t *testing.T) {
	t.Parallel()

	store := &fakeJobStore{}
	objectStore := &fakeObjectStore{
		session: MultipartUploadSession{
			S3Key:    "uploads/job-123/workout.mp4",
			UploadID: "upload-123",
			PartSize: minMultipartPartSize,
			Parts: []UploadPart{
				{PartNumber: 1, UploadURL: "https://example.test/part1"},
				{PartNumber: 2, UploadURL: "https://example.test/part2"},
			},
		},
	}
	router := newTestRouter(store, objectStore)

	body := mustJSON(t, CreateUploadRequest{
		FileName:      "workout.mp4",
		FileSizeBytes: 944075,
		ContentType:   "video/mp4",
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/uploads", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, recorder.Code)
	}
	if len(store.createdUploadJobs) != 1 {
		t.Fatalf("expected 1 created upload job, got %d", len(store.createdUploadJobs))
	}

	createdJob := store.createdUploadJobs[0]
	if createdJob.Status != StatusUploading {
		t.Fatalf("expected status %q, got %q", StatusUploading, createdJob.Status)
	}
	if createdJob.UploadID != "upload-123" {
		t.Fatalf("expected upload id %q, got %q", "upload-123", createdJob.UploadID)
	}
	if createdJob.S3Key != "uploads/job-123/workout.mp4" {
		t.Fatalf("expected s3_key %q, got %q", "uploads/job-123/workout.mp4", createdJob.S3Key)
	}
	if objectStore.lastCreateName != "workout.mp4" || objectStore.lastCreateType != "video/mp4" {
		t.Fatalf("unexpected object store request: %#v", objectStore)
	}

	var response CreateUploadResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Status != StatusUploading {
		t.Fatalf("expected response status %q, got %q", StatusUploading, response.Status)
	}
	if len(response.Parts) != 2 {
		t.Fatalf("expected 2 presigned parts, got %d", len(response.Parts))
	}

	var raw map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode raw response: %v", err)
	}
	if _, exists := raw["s3_key"]; exists {
		t.Fatalf("s3_key should not be exposed: %#v", raw)
	}
}

func TestHandleFinalizeUploadSuccess(t *testing.T) {
	t.Parallel()

	store := &fakeJobStore{
		getJob: JobRecord{
			JobID:         "job-123",
			Status:        StatusUploading,
			FileName:      "workout.mp4",
			FileSizeBytes: 944075,
			ContentType:   "video/mp4",
			S3Key:         "uploads/job-123/workout.mp4",
			UploadID:      "upload-123",
			CreatedAt:     "2026-04-17T00:00:00.000Z",
			FailureType:   FailureTypeNone,
		},
	}
	objectStore := &fakeObjectStore{}
	router := newTestRouter(store, objectStore)

	body := mustJSON(t, FinalizeUploadRequest{
		Parts: []UploadPart{
			{PartNumber: 2, ETag: "\"etag-2\""},
			{PartNumber: 1, ETag: "\"etag-1\""},
		},
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/jobs/job-123/finalize", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, recorder.Code)
	}
	if objectStore.completedKey != "uploads/job-123/workout.mp4" || objectStore.completedUpload != "upload-123" {
		t.Fatalf("unexpected completed upload call: %#v", objectStore)
	}
	if len(store.finalizedUploadJobs) != 1 {
		t.Fatalf("expected 1 finalized job, got %d", len(store.finalizedUploadJobs))
	}
	if store.finalizedUploadJobs[0].Status != StatusQueued {
		t.Fatalf("expected finalized status %q, got %q", StatusQueued, store.finalizedUploadJobs[0].Status)
	}
}

func TestHandleCreateUploadAbortsSessionOnStoreFailure(t *testing.T) {
	t.Parallel()

	store := &fakeJobStore{createUploadErr: errors.New("write failed")}
	objectStore := &fakeObjectStore{
		session: MultipartUploadSession{
			S3Key:    "uploads/job-123/workout.mp4",
			UploadID: "upload-123",
			PartSize: minMultipartPartSize,
			Parts: []UploadPart{
				{PartNumber: 1, UploadURL: "https://example.test/part1"},
			},
		},
	}
	router := newTestRouter(store, objectStore)

	body := mustJSON(t, CreateUploadRequest{
		FileName:      "workout.mp4",
		FileSizeBytes: 944075,
		ContentType:   "video/mp4",
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/uploads", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, recorder.Code)
	}
	if objectStore.abortedKey != "uploads/job-123/workout.mp4" || objectStore.abortedUpload != "upload-123" {
		t.Fatalf("expected aborted upload session, got %#v", objectStore)
	}
}

func TestHandleGetJobSuccess(t *testing.T) {
	t.Parallel()

	store := &fakeJobStore{
		getJob: JobRecord{
			JobID:               "job-123",
			Status:              StatusCompleted,
			FileName:            "workout.mp4",
			FileSizeBytes:       944075,
			CreatedAt:           "2026-04-16T20:00:00.000Z",
			CompletedAt:         "2026-04-16T20:00:03.000Z",
			WorkerID:            "worker-123",
			RetryCount:          2,
			FailureType:         FailureTypeStaleTimeout,
			LastHeartbeatAt:     "2026-04-16T20:00:02.000Z",
			ProcessingStartedAt: "2026-04-16T20:00:01.000Z",
			ResultJSON:          `{"duration_seconds":5.302,"simulated_analysis_seconds":1.06}`,
		},
	}
	router := newTestRouter(store, &fakeObjectStore{})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/jobs/job-123", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}

	var response struct {
		JobID  string         `json:"job_id"`
		Status string         `json:"status"`
		Result map[string]any `json:"result"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.JobID != "job-123" || response.Status != StatusCompleted {
		t.Fatalf("unexpected response: %#v", response)
	}

	var raw map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode raw response: %v", err)
	}
	if _, exists := raw["worker_id"]; exists {
		t.Fatalf("worker_id should not be exposed: %#v", raw)
	}
	if _, exists := raw["retry_count"]; exists {
		t.Fatalf("retry_count should not be exposed: %#v", raw)
	}
	if _, exists := raw["failure_type"]; exists {
		t.Fatalf("failure_type should not be exposed: %#v", raw)
	}
}

func TestHandleGetJobNotFound(t *testing.T) {
	t.Parallel()

	store := &fakeJobStore{getErr: ErrJobNotFound}
	router := newTestRouter(store, &fakeObjectStore{})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/jobs/missing", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, recorder.Code)
	}
}

func TestHandleHealthAndMetrics(t *testing.T) {
	t.Parallel()

	store := &fakeJobStore{
		metrics: MetricsResponse{
			JobsSubmitted:        10,
			JobsCompleted:        8,
			PendingQueueDepth:    2,
			ProcessingQueueDepth: 1,
		},
	}
	router := newTestRouter(store, &fakeObjectStore{})

	healthRecorder := httptest.NewRecorder()
	router.ServeHTTP(healthRecorder, httptest.NewRequest(http.MethodGet, "/health", nil))
	if healthRecorder.Code != http.StatusOK {
		t.Fatalf("expected health status %d, got %d", http.StatusOK, healthRecorder.Code)
	}

	metricsRecorder := httptest.NewRecorder()
	router.ServeHTTP(metricsRecorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if metricsRecorder.Code != http.StatusOK {
		t.Fatalf("expected metrics status %d, got %d", http.StatusOK, metricsRecorder.Code)
	}
}

func mustJSON(t *testing.T, payload any) []byte {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	return body
}
