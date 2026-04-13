package models

// Public photo lifecycle values returned by the API.
const (
	PhotoStatusProcessing = "processing"
	PhotoStatusCompleted  = "completed"
	PhotoStatusFailed     = "failed"
)

// Internal job lifecycle values used by the outbox and worker flow.
const (
	JobStatusPending    = "pending"
	JobStatusPublished  = "published"
	JobStatusProcessing = "processing"
	JobStatusCompleted  = "completed"
	JobStatusFailed     = "failed"
)

// Album is the API contract shape for album create, read, and list responses.
type Album struct {
	AlbumID     string `json:"album_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Owner       string `json:"owner"`
}

// Photo is the internal source-of-truth record stored in Postgres.
type Photo struct {
	PhotoID string
	AlbumID string
	Seq     int
	Status  string
	S3Key   string
	TempPath string
	ContentType string
	URL     string
	Deleted bool
}

// PhotoStatusResponse is returned by GET /albums/:album_id/photos/:photo_id.
type PhotoStatusResponse struct {
	PhotoID string `json:"photo_id"`
	AlbumID string `json:"album_id"`
	Seq     int    `json:"seq"`
	Status  string `json:"status"`
	URL     string `json:"url,omitempty"`
}

// PhotoAcceptedResponse is returned immediately when an upload is accepted.
type PhotoAcceptedResponse struct {
	PhotoID string `json:"photo_id"`
	Seq     int    `json:"seq"`
	Status  string `json:"status"`
}

// PhotoJob tracks async work between the API, outbox publisher, and worker.
type PhotoJob struct {
	JobID    string
	PhotoID  string
	Status   string
	Attempts int
}

// PhotoJobMessage is the minimal queue payload used by SQS.
type PhotoJobMessage struct {
	PhotoID string `json:"photo_id"`
}

// ErrorResponse is the common JSON error body used by the API.
type ErrorResponse struct {
	Error string `json:"error"`
}

// StatusResponse maps the internal photo row to the public photo status payload.
func (p Photo) StatusResponse() PhotoStatusResponse {
	response := PhotoStatusResponse{
		PhotoID: p.PhotoID,
		AlbumID: p.AlbumID,
		Seq:     p.Seq,
		Status:  p.Status,
	}

	if p.Status == PhotoStatusCompleted {
		response.URL = p.URL
	}

	return response
}
