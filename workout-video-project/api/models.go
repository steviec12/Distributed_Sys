package main

const (
	StatusUploading  = "uploading"
	StatusQueued     = "queued"
	StatusInProgress = "in_progress"
	StatusCompleted  = "completed"
	StatusFailed     = "failed"

	FailureTypeNone          = "none"
	FailureTypeStaleTimeout  = "stale_timeout"
	FailureTypeProcessingErr = "processing_error"
	FailureTypePoisonPill    = "poison_pill"
	PendingQueueKey          = "queue:pending"
	ProcessingQueueKey       = "queue:processing"
	RetryableFailedSetKey    = "set:retryable_failed"
	StatsJobsSubmitted       = "stats:jobs_submitted"
	StatsJobsCompleted       = "stats:jobs_completed"
	StatsJobsFailed          = "stats:jobs_failed"
	StatsJobsRejected        = "stats:jobs_rejected"
	StatsJobsRecoveredStale  = "stats:jobs_recovered_stale"
	StatsJobsRecoveredFailed = "stats:jobs_recovered_failed"
	StatsJobsPoisonPill      = "stats:jobs_poison_pill"
)

type JobRecord struct {
	JobID               string
	Status              string
	FileName            string
	FileSizeBytes       int64
	ContentType         string
	S3Key               string
	UploadID            string
	CreatedAt           string
	StartedAt           string
	CompletedAt         string
	ProcessingStartedAt string
	LastHeartbeatAt     string
	WorkerID            string
	RetryCount          int64
	FailureType         string
	Error               string
	ResultJSON          string
}

type JobResponse struct {
	JobID               string  `json:"job_id"`
	Status              string  `json:"status"`
	FileName            string  `json:"file_name"`
	FileSizeBytes       int64   `json:"file_size_bytes"`
	CreatedAt           string  `json:"created_at"`
	ProcessingStartedAt *string `json:"processing_started_at"`
	CompletedAt         *string `json:"completed_at"`
	Error               *string `json:"error"`
	Result              any     `json:"result"`
}

type MetricsResponse struct {
	JobsSubmitted        int64 `json:"jobs_submitted"`
	JobsCompleted        int64 `json:"jobs_completed"`
	JobsFailed           int64 `json:"jobs_failed"`
	JobsRejected         int64 `json:"jobs_rejected"`
	JobsRecoveredStale   int64 `json:"jobs_recovered_stale"`
	JobsRecoveredFailed  int64 `json:"jobs_recovered_failed"`
	JobsPoisonPill       int64 `json:"jobs_poison_pill"`
	PendingQueueDepth    int64 `json:"pending_queue_depth"`
	ProcessingQueueDepth int64 `json:"processing_queue_depth"`
}

type CreateUploadRequest struct {
	FileName      string `json:"file_name"`
	FileSizeBytes int64  `json:"file_size_bytes"`
	ContentType   string `json:"content_type"`
}

type UploadPart struct {
	PartNumber int32  `json:"part_number"`
	UploadURL  string `json:"upload_url,omitempty"`
	ETag       string `json:"etag,omitempty"`
}

type CreateUploadResponse struct {
	JobID    string       `json:"job_id"`
	Status   string       `json:"status"`
	FileName string       `json:"file_name"`
	PartSize int64        `json:"part_size"`
	Parts    []UploadPart `json:"parts"`
}

type FinalizeUploadRequest struct {
	Parts []UploadPart `json:"parts"`
}
