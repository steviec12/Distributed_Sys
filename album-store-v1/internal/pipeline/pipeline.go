package pipeline

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"time"

	"album-store-v1/internal/models"
	"album-store-v1/internal/store"
)

const (
	defaultBufferSize       = 4096
	defaultRecoveryInterval = 10 * time.Second
	defaultRetryBackoff     = 500 * time.Millisecond
)

// PhotoStore captures the photo lifecycle operations used by the in-process workers.
type PhotoStore interface {
	GetPhotoByID(ctx context.Context, photoID string) (models.Photo, error)
	MarkPhotoCompleted(ctx context.Context, albumID, photoID, url string) error
	MarkPhotoFailed(ctx context.Context, albumID, photoID string) error
	ListRecoverablePhotos(ctx context.Context) ([]string, error)
}

// ObjectStorage captures the S3 operations needed during async completion.
type ObjectStorage interface {
	PutObject(ctx context.Context, key string, body io.Reader, contentLength int64, contentType string) error
	DeleteObject(ctx context.Context, key string) error
	PublicURL(key string) string
}

// TempFileStorage captures shared local-file operations between the API and pipeline.
type TempFileStorage interface {
	Open(path string) (io.ReadCloser, error)
	Delete(path string) error
}

// Pipeline is an in-memory, single-process async queue for photo completion jobs.
type Pipeline struct {
	Photos    PhotoStore
	Objects   ObjectStorage
	TempFiles TempFileStorage

	jobs chan string

	mu       sync.Mutex
	enqueued map[string]struct{}
	wg       sync.WaitGroup
	done     chan struct{}
	stopOnce sync.Once
}

// New constructs a pipeline with a buffered in-memory queue.
func New(photos PhotoStore, objects ObjectStorage, tempFiles TempFileStorage, bufferSize int) *Pipeline {
	if bufferSize <= 0 {
		bufferSize = defaultBufferSize
	}

	return &Pipeline{
		Photos:    photos,
		Objects:   objects,
		TempFiles: tempFiles,
		jobs:      make(chan string, bufferSize),
		enqueued:  make(map[string]struct{}),
		done:      make(chan struct{}),
	}
}

// Start launches the in-process worker pool and background stale-photo recovery.
func (p *Pipeline) Start(ctx context.Context, numWorkers int) {
	if numWorkers <= 0 {
		numWorkers = 1
	}

	for i := 0; i < numWorkers; i++ {
		p.wg.Add(1)
		go func(workerID int) {
			defer p.wg.Done()
			p.runWorker(ctx, workerID)
		}(i + 1)
	}

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		p.runRecoveryLoop(ctx)
	}()
}

// Stop waits for all worker goroutines to exit after ctx cancellation.
func (p *Pipeline) Stop() {
	p.stopOnce.Do(func() {
		close(p.done)
	})
	p.wg.Wait()
}

// Submit queues one photo ID for async completion. Duplicate submissions collapse.
func (p *Pipeline) Submit(photoID string) error {
	if photoID == "" {
		return fmt.Errorf("photo id is required")
	}
	select {
	case <-p.done:
		return fmt.Errorf("pipeline stopped")
	default:
	}

	if !p.markEnqueued(photoID) {
		return nil
	}

	select {
	case p.jobs <- photoID:
		return nil
	default:
		go func(id string) {
			select {
			case <-p.done:
				p.clearEnqueued(id)
			case p.jobs <- id:
			}
		}(photoID)
		return nil
	}
}

// RecoverStale requeues any processing photos that still have a temp file path.
func (p *Pipeline) RecoverStale(ctx context.Context) error {
	photoIDs, err := p.Photos.ListRecoverablePhotos(ctx)
	if err != nil {
		return fmt.Errorf("list recoverable photos: %w", err)
	}

	for _, photoID := range photoIDs {
		if err := p.Submit(photoID); err != nil {
			return fmt.Errorf("submit recoverable photo %s: %w", photoID, err)
		}
	}

	return nil
}

func (p *Pipeline) runWorker(ctx context.Context, workerID int) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-p.done:
			return
		case photoID := <-p.jobs:
			if photoID == "" {
				continue
			}
			if err := p.processPhoto(ctx, photoID); err != nil {
				log.Printf("pipeline worker=%d retry photo_id=%s err=%v", workerID, photoID, err)
				p.clearEnqueued(photoID)
				select {
				case <-ctx.Done():
					return
				case <-time.After(defaultRetryBackoff):
				}
				if err := p.Submit(photoID); err != nil {
					log.Printf("pipeline worker=%d requeue failed photo_id=%s err=%v", workerID, photoID, err)
				}
				continue
			}
			p.clearEnqueued(photoID)
		}
	}
}

func (p *Pipeline) runRecoveryLoop(ctx context.Context) {
	ticker := time.NewTicker(defaultRecoveryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := p.RecoverStale(ctx); err != nil {
				log.Printf("pipeline recover stale error: %v", err)
			}
		}
	}
}

func (p *Pipeline) processPhoto(ctx context.Context, photoID string) error {
	photo, err := p.Photos.GetPhotoByID(ctx, photoID)
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load photo %s: %w", photoID, err)
	}

	if photo.Status == models.PhotoStatusCompleted {
		return nil
	}

	if photo.TempPath == "" {
		if err := p.Photos.MarkPhotoFailed(ctx, photo.AlbumID, photo.PhotoID); err != nil && !errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("mark photo failed %s: %w", photo.PhotoID, err)
		}
		return nil
	}

	file, err := p.TempFiles.Open(photo.TempPath)
	if err != nil {
		if err := p.Photos.MarkPhotoFailed(ctx, photo.AlbumID, photo.PhotoID); err != nil && !errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("mark photo failed %s: %w", photo.PhotoID, err)
		}
		return nil
	}
	defer file.Close()

	var contentLength int64
	if statter, ok := file.(interface{ Stat() (os.FileInfo, error) }); ok {
		if info, err := statter.Stat(); err == nil {
			contentLength = info.Size()
		}
	}

	if err := p.Objects.PutObject(ctx, photo.S3Key, file, contentLength, photo.ContentType); err != nil {
		return fmt.Errorf("put object %s: %w", photo.S3Key, err)
	}

	if err := p.Photos.MarkPhotoCompleted(ctx, photo.AlbumID, photo.PhotoID, p.Objects.PublicURL(photo.S3Key)); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			if deleteErr := p.Objects.DeleteObject(ctx, photo.S3Key); deleteErr != nil {
				log.Printf("pipeline deleted-photo object cleanup failed photo_id=%s s3_key=%s err=%v", photo.PhotoID, photo.S3Key, deleteErr)
			}
			_ = p.TempFiles.Delete(photo.TempPath)
			return nil
		}
		return fmt.Errorf("mark photo completed %s: %w", photo.PhotoID, err)
	}

	if err := p.TempFiles.Delete(photo.TempPath); err != nil {
		log.Printf("pipeline temp cleanup failed photo_id=%s temp_path=%s err=%v", photo.PhotoID, photo.TempPath, err)
	}

	return nil
}

func (p *Pipeline) markEnqueued(photoID string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, exists := p.enqueued[photoID]; exists {
		return false
	}

	p.enqueued[photoID] = struct{}{}
	return true
}

func (p *Pipeline) clearEnqueued(photoID string) {
	p.mu.Lock()
	delete(p.enqueued, photoID)
	p.mu.Unlock()
}
