package worker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"album-store-v1/internal/models"
	"album-store-v1/internal/queue"
	"album-store-v1/internal/store"
)

const (
	defaultPublishBatchSize   = 100
	defaultReceiveBatchSize   = 10
	defaultReceiveWaitTime    = 2
	defaultProcessConcurrency = 10
)

var (
	errorBackoff          = time.Second
	publishTickerInterval = 200 * time.Millisecond
	processConcurrency    = defaultProcessConcurrency
)

// JobStore captures the outbox and job-state operations used by the worker.
type JobStore interface {
	ListPendingPhotoJobs(ctx context.Context, limit int) ([]models.PhotoJob, error)
	MarkPhotoJobPublished(ctx context.Context, jobID string) error
	MarkPhotoJobCompletedByPhotoID(ctx context.Context, photoID string) error
	MarkPhotoJobFailedByPhotoID(ctx context.Context, photoID string) error
}

// PhotoStore captures the photo state transitions used by the worker.
type PhotoStore interface {
	GetPhotoByID(ctx context.Context, photoID string) (models.Photo, error)
	MarkPhotoCompleted(ctx context.Context, albumID, photoID, url string) error
	MarkPhotoFailed(ctx context.Context, albumID, photoID string) error
}

// ObjectStorage provides the S3 checks needed while finalizing a photo.
type ObjectStorage interface {
	PutObject(ctx context.Context, key string, body io.Reader, contentLength int64, contentType string) error
	PublicURL(key string) string
}

// TempFileStorage provides access to locally staged uploads on the app host.
type TempFileStorage interface {
	Open(path string) (io.ReadCloser, error)
	Delete(path string) error
}

// Service owns the asynchronous side of the system:
// 1. move pending DB jobs into SQS
// 2. consume SQS messages and finalize photo state
type Service struct {
	Jobs    JobStore
	Photos  PhotoStore
	Objects ObjectStorage
	TempFiles TempFileStorage
	Queue   queue.PhotoJobConsumer
	Publish queue.PhotoJobPublisher
}

// PublishPendingJobs moves durable outbox rows into SQS.
func (s Service) PublishPendingJobs(ctx context.Context, limit int) error {
	jobs, err := s.Jobs.ListPendingPhotoJobs(ctx, limit)
	if err != nil {
		return fmt.Errorf("list pending jobs: %w", err)
	}
	if len(jobs) > 0 {
		log.Printf("worker publish batch pending_jobs=%d limit=%d", len(jobs), limit)
	}

	var publishErrs []error
	for _, job := range jobs {
		// Pending DB jobs are the durable outbox. Once publish succeeds, the row
		// moves to published so the worker can rely on SQS for delivery.
		if err := s.Publish.PublishPhotoJob(ctx, models.PhotoJobMessage{PhotoID: job.PhotoID}); err != nil {
			log.Printf("worker publish failed job_id=%s photo_id=%s err=%v", job.JobID, job.PhotoID, err)
			publishErrs = append(publishErrs, fmt.Errorf("publish job %s: %w", job.JobID, err))
			continue
		}
		if err := s.Jobs.MarkPhotoJobPublished(ctx, job.JobID); err != nil {
			log.Printf("worker mark published failed job_id=%s photo_id=%s err=%v", job.JobID, job.PhotoID, err)
			publishErrs = append(publishErrs, fmt.Errorf("mark job %s published: %w", job.JobID, err))
			continue
		}
		log.Printf("worker published job_id=%s photo_id=%s", job.JobID, job.PhotoID)
	}

	return errors.Join(publishErrs...)
}

// ProcessQueueBatch handles one receive cycle from SQS.
func (s Service) ProcessQueueBatch(ctx context.Context, maxMessages int32, waitTimeSeconds int32) error {
	messages, err := s.Queue.ReceivePhotoJobs(ctx, maxMessages, waitTimeSeconds)
	if err != nil {
		return fmt.Errorf("receive queue messages: %w", err)
	}
	if len(messages) > 0 {
		log.Printf("worker received messages=%d wait_time_seconds=%d", len(messages), waitTimeSeconds)
	}

	var processErrs []error
	var mu sync.Mutex
	sem := make(chan struct{}, processConcurrency)
	var wg sync.WaitGroup
	for _, message := range messages {
		message := message
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()

			if err := s.processMessage(ctx, message); err != nil {
				mu.Lock()
				processErrs = append(processErrs, err)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	return errors.Join(processErrs...)
}

// processMessage finalizes one photo or marks it failed, then acknowledges SQS.
func (s Service) processMessage(ctx context.Context, message queue.ReceivedPhotoJob) error {
	log.Printf("worker process start photo_id=%s receipt_handle_present=%t", message.Message.PhotoID, message.ReceiptHandle != "")

	photo, err := s.Photos.GetPhotoByID(ctx, message.Message.PhotoID)
	if errors.Is(err, store.ErrNotFound) {
		// Missing/deleted photos are terminal. We acknowledge the queue message
		// so a deleted photo cannot be resurrected by repeated delivery.
		_ = s.Jobs.MarkPhotoJobCompletedByPhotoID(ctx, message.Message.PhotoID)
		log.Printf("worker process photo missing photo_id=%s treating_as_terminal=true", message.Message.PhotoID)
		return s.Queue.DeleteMessage(ctx, message.ReceiptHandle)
	}
	if err != nil {
		return fmt.Errorf("load photo %s: %w", message.Message.PhotoID, err)
	}

	if photo.Status == models.PhotoStatusCompleted {
		_ = s.Jobs.MarkPhotoJobCompletedByPhotoID(ctx, photo.PhotoID)
		log.Printf("worker process already completed album_id=%s photo_id=%s s3_key=%s", photo.AlbumID, photo.PhotoID, photo.S3Key)
		return s.Queue.DeleteMessage(ctx, message.ReceiptHandle)
	}

	if photo.TempPath == "" {
		if err := s.Photos.MarkPhotoFailed(ctx, photo.AlbumID, photo.PhotoID); err != nil && !errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("mark photo failed %s: %w", photo.PhotoID, err)
		}
		_ = s.Jobs.MarkPhotoJobFailedByPhotoID(ctx, photo.PhotoID)
		log.Printf("worker process temp file missing album_id=%s photo_id=%s s3_key=%s marked_failed=true", photo.AlbumID, photo.PhotoID, photo.S3Key)
		return s.Queue.DeleteMessage(ctx, message.ReceiptHandle)
	}

	file, err := s.TempFiles.Open(photo.TempPath)
	if err != nil {
		if err := s.Photos.MarkPhotoFailed(ctx, photo.AlbumID, photo.PhotoID); err != nil && !errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("mark photo failed %s: %w", photo.PhotoID, err)
		}
		_ = s.Jobs.MarkPhotoJobFailedByPhotoID(ctx, photo.PhotoID)
		log.Printf("worker process open temp file failed album_id=%s photo_id=%s temp_path=%s err=%v marked_failed=true", photo.AlbumID, photo.PhotoID, photo.TempPath, err)
		return s.Queue.DeleteMessage(ctx, message.ReceiptHandle)
	}
	defer file.Close()

	if err := s.Objects.PutObject(ctx, photo.S3Key, file, 0, photo.ContentType); err != nil {
		return fmt.Errorf("upload object %s: %w", photo.S3Key, err)
	}

	if err := s.Photos.MarkPhotoCompleted(ctx, photo.AlbumID, photo.PhotoID, s.Objects.PublicURL(photo.S3Key)); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			_ = s.Jobs.MarkPhotoJobCompletedByPhotoID(ctx, photo.PhotoID)
			log.Printf("worker process completion target missing album_id=%s photo_id=%s treating_as_terminal=true", photo.AlbumID, photo.PhotoID)
			return s.Queue.DeleteMessage(ctx, message.ReceiptHandle)
		}
		return fmt.Errorf("mark photo completed %s: %w", photo.PhotoID, err)
	}

	if err := s.Jobs.MarkPhotoJobCompletedByPhotoID(ctx, photo.PhotoID); err != nil && !errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("mark job completed for photo %s: %w", photo.PhotoID, err)
	}
	if err := s.TempFiles.Delete(photo.TempPath); err != nil {
		log.Printf("worker process temp cleanup failed album_id=%s photo_id=%s temp_path=%s err=%v", photo.AlbumID, photo.PhotoID, photo.TempPath, err)
	}
	log.Printf("worker process completed album_id=%s photo_id=%s s3_key=%s temp_path=%s", photo.AlbumID, photo.PhotoID, photo.S3Key, photo.TempPath)

	return s.Queue.DeleteMessage(ctx, message.ReceiptHandle)
}

// Run alternates between polling SQS and publishing any pending outbox rows.
func (s Service) Run(ctx context.Context) error {
	publishTicker := time.NewTicker(publishTickerInterval)
	defer publishTicker.Stop()

	for {
		if err := s.ProcessQueueBatch(ctx, defaultReceiveBatchSize, defaultReceiveWaitTime); err != nil {
			log.Printf("worker queue batch error: %v", err)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(errorBackoff):
			}
			continue
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-publishTicker.C:
			if err := s.PublishPendingJobs(ctx, defaultPublishBatchSize); err != nil {
				log.Printf("worker publish error: %v", err)
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(errorBackoff):
				}
			}
		default:
		}
	}
}
