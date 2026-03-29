package main

const (
	StatusQueued     = "queued"
	StatusInProgress = "in_progress"
	StatusCompleted  = "completed"
	StatusFailed     = "failed"

	PendingQueueKey    = "queue:pending"
	StatsJobsSubmitted = "stats:jobs_submitted"
	StatsJobsCompleted = "stats:jobs_completed"
	StatsJobsFailed    = "stats:jobs_failed"
)

type JobRecord struct {
	JobID         string
	Status        string
	FilePath      string
	FileName      string
	FileSizeBytes int64
	CreatedAt     string
	StartedAt     string
	CompletedAt   string
	Error         string
	ResultJSON    string
}

type JobResponse struct {
	JobID         string  `json:"job_id"`
	Status        string  `json:"status"`
	FileName      string  `json:"file_name"`
	FileSizeBytes int64   `json:"file_size_bytes"`
	CreatedAt     string  `json:"created_at"`
	StartedAt     *string `json:"started_at"`
	CompletedAt   *string `json:"completed_at"`
	Error         *string `json:"error"`
	Result        any     `json:"result"`
}
