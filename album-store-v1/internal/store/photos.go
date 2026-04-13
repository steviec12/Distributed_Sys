package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"album-store-v1/internal/models"
)

const allocatePhotoSeqSQL = `
UPDATE albums
SET next_seq = next_seq + 1
WHERE album_id = $1
RETURNING next_seq - 1`

const insertPhotoSQL = `
INSERT INTO photos (photo_id, album_id, seq, status, s3_key, temp_path, content_type, url, deleted)
VALUES ($1, $2, $3, 'processing', $4, $5, $6, NULL, FALSE)
RETURNING photo_id, album_id, seq, status, s3_key, COALESCE(temp_path, ''), COALESCE(content_type, ''), COALESCE(url, ''), deleted`

const insertPhotoJobSQL = `
INSERT INTO photo_jobs (job_id, photo_id, status, attempts)
VALUES ($1, $2, 'pending', 0)`

const getPhotoSQL = `
SELECT photo_id, album_id, seq, status, s3_key, COALESCE(temp_path, ''), COALESCE(content_type, ''), COALESCE(url, ''), deleted
FROM photos
WHERE album_id = $1
  AND photo_id = $2
  AND deleted = FALSE`

const getPhotoForDeleteSQL = `
SELECT photo_id, album_id, seq, status, s3_key, COALESCE(temp_path, ''), COALESCE(content_type, ''), COALESCE(url, ''), deleted
FROM photos
WHERE album_id = $1
  AND photo_id = $2`

const getPhotoByIDSQL = `
SELECT photo_id, album_id, seq, status, s3_key, COALESCE(temp_path, ''), COALESCE(content_type, ''), COALESCE(url, ''), deleted
FROM photos
WHERE photo_id = $1
  AND deleted = FALSE`

const listRecoverablePhotosSQL = `
SELECT photo_id
FROM photos
WHERE status = 'processing'
  AND deleted = FALSE
  AND temp_path IS NOT NULL
ORDER BY photo_id`

const markPhotoCompletedSQL = `
UPDATE photos
SET status = 'completed',
    temp_path = NULL,
    content_type = NULL,
    url = $3
WHERE album_id = $1
  AND photo_id = $2
  AND deleted = FALSE`

const markPhotoFailedSQL = `
UPDATE photos
SET status = 'failed',
    temp_path = NULL,
    content_type = NULL,
    url = NULL
WHERE album_id = $1
  AND photo_id = $2
  AND deleted = FALSE`

const markPhotoDeletedSQL = `
UPDATE photos
SET deleted = TRUE
WHERE album_id = $1
  AND photo_id = $2
  AND deleted = FALSE
RETURNING photo_id, album_id, seq, status, s3_key, COALESCE(temp_path, ''), COALESCE(content_type, ''), COALESCE(url, ''), deleted`

const markPhotoJobPublishedSQL = `
UPDATE photo_jobs
SET status = 'published'
WHERE job_id = $1
  AND status = 'pending'`

const listPendingPhotoJobsSQL = `
SELECT job_id, photo_id, status, attempts
FROM photo_jobs
WHERE status = 'pending'
ORDER BY job_id
LIMIT $1`

const markPhotoJobCompletedByPhotoIDSQL = `
UPDATE photo_jobs
SET status = 'completed'
WHERE photo_id = $1`

const markPhotoJobFailedByPhotoIDSQL = `
UPDATE photo_jobs
SET status = 'failed'
WHERE photo_id = $1`

// CreatePhotoProcessing allocates seq and inserts a photo row without a job.
func (s *Store) CreatePhotoProcessing(ctx context.Context, albumID, photoID, s3Key, tempPath, contentType string) (models.Photo, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return models.Photo{}, fmt.Errorf("begin create photo transaction: %w", err)
	}
	defer tx.Rollback()

	var seq int
	err = tx.QueryRowContext(ctx, allocatePhotoSeqSQL, albumID).Scan(&seq)
	if errors.Is(err, sql.ErrNoRows) {
		return models.Photo{}, ErrNotFound
	}
	if err != nil {
		return models.Photo{}, fmt.Errorf("allocate photo sequence: %w", err)
	}

	var photo models.Photo
	err = tx.QueryRowContext(ctx, insertPhotoSQL, photoID, albumID, seq, s3Key, tempPath, contentType).
		Scan(&photo.PhotoID, &photo.AlbumID, &photo.Seq, &photo.Status, &photo.S3Key, &photo.TempPath, &photo.ContentType, &photo.URL, &photo.Deleted)
	if err != nil {
		return models.Photo{}, fmt.Errorf("insert photo: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return models.Photo{}, fmt.Errorf("commit create photo transaction: %w", err)
	}

	return photo, nil
}

// CreatePhotoProcessingJob stores both the accepted photo row and its pending
// outbox job in one transaction so accepted work is durable before 202.
func (s *Store) CreatePhotoProcessingJob(ctx context.Context, albumID, photoID, jobID, s3Key, tempPath, contentType string) (models.Photo, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return models.Photo{}, fmt.Errorf("begin create photo job transaction: %w", err)
	}
	defer tx.Rollback()

	var seq int
	err = tx.QueryRowContext(ctx, allocatePhotoSeqSQL, albumID).Scan(&seq)
	if errors.Is(err, sql.ErrNoRows) {
		return models.Photo{}, ErrNotFound
	}
	if err != nil {
		return models.Photo{}, fmt.Errorf("allocate photo sequence: %w", err)
	}

	// The photo row and pending outbox job are committed together so the API
	// can acknowledge accepted work without depending on SQS in that request.
	var photo models.Photo
	err = tx.QueryRowContext(ctx, insertPhotoSQL, photoID, albumID, seq, s3Key, tempPath, contentType).
		Scan(&photo.PhotoID, &photo.AlbumID, &photo.Seq, &photo.Status, &photo.S3Key, &photo.TempPath, &photo.ContentType, &photo.URL, &photo.Deleted)
	if err != nil {
		return models.Photo{}, fmt.Errorf("insert photo: %w", err)
	}

	if _, err := tx.ExecContext(ctx, insertPhotoJobSQL, jobID, photoID); err != nil {
		return models.Photo{}, fmt.Errorf("insert photo job: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return models.Photo{}, fmt.Errorf("commit create photo job transaction: %w", err)
	}

	return photo, nil
}

// GetPhoto loads one visible photo. Tombstoned rows are treated as missing.
func (s *Store) GetPhoto(ctx context.Context, albumID, photoID string) (models.Photo, error) {
	var photo models.Photo
	err := s.db.QueryRowContext(ctx, getPhotoSQL, albumID, photoID).
		Scan(&photo.PhotoID, &photo.AlbumID, &photo.Seq, &photo.Status, &photo.S3Key, &photo.TempPath, &photo.ContentType, &photo.URL, &photo.Deleted)
	if errors.Is(err, sql.ErrNoRows) {
		return models.Photo{}, ErrNotFound
	}
	if err != nil {
		return models.Photo{}, fmt.Errorf("get photo: %w", err)
	}

	return photo, nil
}

// GetPhotoForDelete loads a photo even after tombstoning so delete retries can
// still reach the stored S3 key and finish object cleanup.
func (s *Store) GetPhotoForDelete(ctx context.Context, albumID, photoID string) (models.Photo, error) {
	var photo models.Photo
	err := s.db.QueryRowContext(ctx, getPhotoForDeleteSQL, albumID, photoID).
		Scan(&photo.PhotoID, &photo.AlbumID, &photo.Seq, &photo.Status, &photo.S3Key, &photo.TempPath, &photo.ContentType, &photo.URL, &photo.Deleted)
	if errors.Is(err, sql.ErrNoRows) {
		return models.Photo{}, ErrNotFound
	}
	if err != nil {
		return models.Photo{}, fmt.Errorf("get photo for delete: %w", err)
	}

	return photo, nil
}

// GetPhotoByID is the worker-facing lookup used from SQS payloads.
func (s *Store) GetPhotoByID(ctx context.Context, photoID string) (models.Photo, error) {
	var photo models.Photo
	err := s.db.QueryRowContext(ctx, getPhotoByIDSQL, photoID).
		Scan(&photo.PhotoID, &photo.AlbumID, &photo.Seq, &photo.Status, &photo.S3Key, &photo.TempPath, &photo.ContentType, &photo.URL, &photo.Deleted)
	if errors.Is(err, sql.ErrNoRows) {
		return models.Photo{}, ErrNotFound
	}
	if err != nil {
		return models.Photo{}, fmt.Errorf("get photo by id: %w", err)
	}

	return photo, nil
}

// ListRecoverablePhotos returns processing rows that still point at a staged temp file.
func (s *Store) ListRecoverablePhotos(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, listRecoverablePhotosSQL)
	if err != nil {
		return nil, fmt.Errorf("list recoverable photos: %w", err)
	}
	defer rows.Close()

	var photoIDs []string
	for rows.Next() {
		var photoID string
		if err := rows.Scan(&photoID); err != nil {
			return nil, fmt.Errorf("scan recoverable photo: %w", err)
		}
		photoIDs = append(photoIDs, photoID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate recoverable photos: %w", err)
	}

	return photoIDs, nil
}

// MarkPhotoCompleted finalizes a visible photo with its public URL.
func (s *Store) MarkPhotoCompleted(ctx context.Context, albumID, photoID, url string) error {
	return s.execPhotoMutation(ctx, markPhotoCompletedSQL, "mark photo completed", albumID, photoID, url)
}

// MarkPhotoFailed moves a visible photo into the contract's failed state.
func (s *Store) MarkPhotoFailed(ctx context.Context, albumID, photoID string) error {
	return s.execPhotoMutation(ctx, markPhotoFailedSQL, "mark photo failed", albumID, photoID)
}

// MarkPhotoDeleted tombstones the photo so GET starts returning 404 immediately.
func (s *Store) MarkPhotoDeleted(ctx context.Context, albumID, photoID string) (models.Photo, error) {
	var photo models.Photo
	err := s.db.QueryRowContext(ctx, markPhotoDeletedSQL, albumID, photoID).
		Scan(&photo.PhotoID, &photo.AlbumID, &photo.Seq, &photo.Status, &photo.S3Key, &photo.TempPath, &photo.ContentType, &photo.URL, &photo.Deleted)
	if errors.Is(err, sql.ErrNoRows) {
		return models.Photo{}, ErrNotFound
	}
	if err != nil {
		return models.Photo{}, fmt.Errorf("mark photo deleted: %w", err)
	}

	return photo, nil
}

// MarkPhotoJobPublished records that a pending outbox row has reached SQS.
func (s *Store) MarkPhotoJobPublished(ctx context.Context, jobID string) error {
	return s.execPhotoMutation(ctx, markPhotoJobPublishedSQL, "mark photo job published", jobID)
}

// MarkPhotoJobCompletedByPhotoID closes the async lifecycle for one photo.
func (s *Store) MarkPhotoJobCompletedByPhotoID(ctx context.Context, photoID string) error {
	return s.execPhotoMutation(ctx, markPhotoJobCompletedByPhotoIDSQL, "mark photo job completed", photoID)
}

// MarkPhotoJobFailedByPhotoID records a terminal worker failure for one photo.
func (s *Store) MarkPhotoJobFailedByPhotoID(ctx context.Context, photoID string) error {
	return s.execPhotoMutation(ctx, markPhotoJobFailedByPhotoIDSQL, "mark photo job failed", photoID)
}

// ListPendingPhotoJobs returns unpublished outbox rows for the publisher loop.
func (s *Store) ListPendingPhotoJobs(ctx context.Context, limit int) ([]models.PhotoJob, error) {
	rows, err := s.db.QueryContext(ctx, listPendingPhotoJobsSQL, limit)
	if err != nil {
		return nil, fmt.Errorf("list pending photo jobs: %w", err)
	}
	defer rows.Close()

	var jobs []models.PhotoJob
	for rows.Next() {
		var job models.PhotoJob
		if err := rows.Scan(&job.JobID, &job.PhotoID, &job.Status, &job.Attempts); err != nil {
			return nil, fmt.Errorf("scan photo job: %w", err)
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate photo jobs: %w", err)
	}

	return jobs, nil
}

// execPhotoMutation centralizes rows-affected checks for state transitions.
func (s *Store) execPhotoMutation(ctx context.Context, query, operation string, args ...any) error {
	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s rows affected: %w", operation, err)
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}
