package worker

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"album-store-v1/internal/models"
	"album-store-v1/internal/queue"
	"album-store-v1/internal/store"
)

type fakeJobStore struct {
	pendingJobs      []models.PhotoJob
	listErr          error
	publishedJobID   string
	publishedCalls   int
	publishedErr     error
	completedPhotoID string
	completedCalls   int
	completedErr     error
	failedPhotoID    string
	failedCalls      int
	failedErr        error
}

func (f *fakeJobStore) ListPendingPhotoJobs(_ context.Context, limit int) ([]models.PhotoJob, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	if len(f.pendingJobs) > limit {
		return f.pendingJobs[:limit], nil
	}
	return f.pendingJobs, nil
}

func (f *fakeJobStore) MarkPhotoJobPublished(_ context.Context, jobID string) error {
	f.publishedCalls++
	f.publishedJobID = jobID
	return f.publishedErr
}

func (f *fakeJobStore) MarkPhotoJobCompletedByPhotoID(_ context.Context, photoID string) error {
	f.completedCalls++
	f.completedPhotoID = photoID
	return f.completedErr
}

func (f *fakeJobStore) MarkPhotoJobFailedByPhotoID(_ context.Context, photoID string) error {
	f.failedCalls++
	f.failedPhotoID = photoID
	return f.failedErr
}

type fakePhotoStoreWorker struct {
	photo          models.Photo
	getErr         error
	getCalls       int
	completedAlbum string
	completedPhoto string
	completedURL   string
	completedCalls int
	completedErr   error
	failedAlbum    string
	failedPhoto    string
	failedCalls    int
	failedErr      error
}

func (f *fakePhotoStoreWorker) GetPhotoByID(_ context.Context, photoID string) (models.Photo, error) {
	f.getCalls++
	if f.getErr != nil {
		return models.Photo{}, f.getErr
	}
	if f.photo.PhotoID == "" {
		f.photo.PhotoID = photoID
	}
	return f.photo, nil
}

func (f *fakePhotoStoreWorker) MarkPhotoCompleted(_ context.Context, albumID, photoID, url string) error {
	f.completedCalls++
	f.completedAlbum = albumID
	f.completedPhoto = photoID
	f.completedURL = url
	return f.completedErr
}

func (f *fakePhotoStoreWorker) MarkPhotoFailed(_ context.Context, albumID, photoID string) error {
	f.failedCalls++
	f.failedAlbum = albumID
	f.failedPhoto = photoID
	return f.failedErr
}

type fakeObjectStorageWorker struct {
	putKey         string
	putContentType string
	putCalls       int
	putErr         error
	publicURL      string
}

func (f *fakeObjectStorageWorker) PutObject(_ context.Context, key string, body io.Reader, _ int64, contentType string) error {
	f.putCalls++
	f.putKey = key
	f.putContentType = contentType
	if body == nil {
		return errors.New("missing body")
	}
	_, _ = io.ReadAll(body)
	return f.putErr
}

func (f *fakeObjectStorageWorker) PublicURL(key string) string {
	if f.publicURL != "" {
		return f.publicURL
	}
	return "https://bucket/" + key
}

type fakeTempFileStorageWorker struct {
	openPath    string
	openCalls   int
	openErr     error
	deletePath  string
	deleteCalls int
	deleteErr   error
}

func (f *fakeTempFileStorageWorker) Open(path string) (io.ReadCloser, error) {
	f.openCalls++
	f.openPath = path
	if f.openErr != nil {
		return nil, f.openErr
	}
	return io.NopCloser(strings.NewReader("file-bytes")), nil
}

func (f *fakeTempFileStorageWorker) Delete(path string) error {
	f.deleteCalls++
	f.deletePath = path
	return f.deleteErr
}

type fakeQueue struct {
	publishMessage models.PhotoJobMessage
	publishCalls   int
	publishErr     error
	publishErrs    []error
	publishHook    func(int)
	messages       []queue.ReceivedPhotoJob
	receiveErr     error
	receiveErrs    []error
	receiveHook    func(int)
	receiveCalls   int
	deletedHandle  string
	deleteCalls    int
	deleteErr      error
}

func (f *fakeQueue) PublishPhotoJob(_ context.Context, message models.PhotoJobMessage) error {
	f.publishCalls++
	f.publishMessage = message
	if f.publishHook != nil {
		f.publishHook(f.publishCalls)
	}
	if len(f.publishErrs) >= f.publishCalls {
		return f.publishErrs[f.publishCalls-1]
	}
	return f.publishErr
}

func (f *fakeQueue) ReceivePhotoJobs(_ context.Context, _ int32, _ int32) ([]queue.ReceivedPhotoJob, error) {
	f.receiveCalls++
	if f.receiveHook != nil {
		f.receiveHook(f.receiveCalls)
	}
	if len(f.receiveErrs) >= f.receiveCalls {
		if err := f.receiveErrs[f.receiveCalls-1]; err != nil {
			return nil, err
		}
	}
	if f.receiveErr != nil {
		return nil, f.receiveErr
	}
	return f.messages, nil
}

func (f *fakeQueue) DeleteMessage(_ context.Context, receiptHandle string) error {
	f.deleteCalls++
	f.deletedHandle = receiptHandle
	return f.deleteErr
}

func TestPublishPendingJobsPublishesAndMarksJobs(t *testing.T) {
	jobs := &fakeJobStore{
		pendingJobs: []models.PhotoJob{{JobID: "job-1", PhotoID: "photo-1", Status: models.JobStatusPending}},
	}
	queue := &fakeQueue{}
	service := Service{Jobs: jobs, Publish: queue}

	if err := service.PublishPendingJobs(context.Background(), 10); err != nil {
		t.Fatalf("publish pending jobs: %v", err)
	}
	if queue.publishCalls != 1 || queue.publishMessage.PhotoID != "photo-1" {
		t.Fatalf("unexpected publish state %+v", queue)
	}
	if jobs.publishedCalls != 1 || jobs.publishedJobID != "job-1" {
		t.Fatalf("unexpected job state %+v", jobs)
	}
}

func TestProcessQueueBatchCompletesPhotoAndDeletesMessage(t *testing.T) {
	jobs := &fakeJobStore{}
	photos := &fakePhotoStoreWorker{
		photo: models.Photo{
			PhotoID:     "photo-1",
			AlbumID:     "album-1",
			S3Key:       "albums/album-1/photos/photo-1.jpg",
			TempPath:    "/tmp/photo-1.jpg",
			ContentType: "image/jpeg",
		},
	}
	objects := &fakeObjectStorageWorker{publicURL: "https://bucket/albums/album-1/photos/photo-1.jpg"}
	tempFiles := &fakeTempFileStorageWorker{}
	queue := &fakeQueue{
		messages: []queue.ReceivedPhotoJob{{
			Message:       models.PhotoJobMessage{PhotoID: "photo-1"},
			ReceiptHandle: "receipt-1",
		}},
	}
	service := Service{Jobs: jobs, Photos: photos, Objects: objects, TempFiles: tempFiles, Queue: queue}

	if err := service.ProcessQueueBatch(context.Background(), 1, 1); err != nil {
		t.Fatalf("process queue batch: %v", err)
	}
	if objects.putCalls != 1 || objects.putKey != "albums/album-1/photos/photo-1.jpg" {
		t.Fatalf("unexpected object upload %+v", objects)
	}
	if photos.completedCalls != 1 || photos.completedURL != "https://bucket/albums/album-1/photos/photo-1.jpg" {
		t.Fatalf("unexpected photo completion %+v", photos)
	}
	if tempFiles.deleteCalls != 1 || tempFiles.deletePath != "/tmp/photo-1.jpg" {
		t.Fatalf("unexpected temp cleanup %+v", tempFiles)
	}
	if jobs.completedCalls != 1 || queue.deleteCalls != 1 || queue.deletedHandle != "receipt-1" {
		t.Fatalf("unexpected terminal state jobs=%+v queue=%+v", jobs, queue)
	}
}

func TestProcessQueueBatchTreatsMissingPhotoAsTerminal(t *testing.T) {
	jobs := &fakeJobStore{}
	photos := &fakePhotoStoreWorker{getErr: store.ErrNotFound}
	queue := &fakeQueue{
		messages: []queue.ReceivedPhotoJob{{
			Message:       models.PhotoJobMessage{PhotoID: "photo-1"},
			ReceiptHandle: "receipt-1",
		}},
	}
	service := Service{Jobs: jobs, Photos: photos, Objects: &fakeObjectStorageWorker{}, TempFiles: &fakeTempFileStorageWorker{}, Queue: queue}

	if err := service.ProcessQueueBatch(context.Background(), 1, 1); err != nil {
		t.Fatalf("process queue batch: %v", err)
	}
	if jobs.completedCalls != 1 || queue.deleteCalls != 1 {
		t.Fatalf("unexpected terminal handling jobs=%+v queue=%+v", jobs, queue)
	}
}

func TestProcessQueueBatchMarksMissingTempPathFailed(t *testing.T) {
	jobs := &fakeJobStore{}
	photos := &fakePhotoStoreWorker{
		photo: models.Photo{
			PhotoID: "photo-1",
			AlbumID: "album-1",
			S3Key:   "albums/album-1/photos/photo-1.jpg",
		},
	}
	queue := &fakeQueue{
		messages: []queue.ReceivedPhotoJob{{
			Message:       models.PhotoJobMessage{PhotoID: "photo-1"},
			ReceiptHandle: "receipt-1",
		}},
	}
	service := Service{
		Jobs:      jobs,
		Photos:    photos,
		Objects:   &fakeObjectStorageWorker{},
		TempFiles: &fakeTempFileStorageWorker{},
		Queue:     queue,
	}

	if err := service.ProcessQueueBatch(context.Background(), 1, 1); err != nil {
		t.Fatalf("process queue batch: %v", err)
	}
	if photos.failedCalls != 1 || jobs.failedCalls != 1 || queue.deleteCalls != 1 {
		t.Fatalf("unexpected missing-temp handling photos=%+v jobs=%+v queue=%+v", photos, jobs, queue)
	}
}

func TestProcessQueueBatchLeavesMessageForRetryOnUploadError(t *testing.T) {
	jobs := &fakeJobStore{}
	photos := &fakePhotoStoreWorker{
		photo: models.Photo{
			PhotoID:     "photo-1",
			AlbumID:     "album-1",
			S3Key:       "albums/album-1/photos/photo-1.jpg",
			TempPath:    "/tmp/photo-1.jpg",
			ContentType: "image/jpeg",
		},
	}
	objects := &fakeObjectStorageWorker{putErr: errors.New("s3 down")}
	queue := &fakeQueue{
		messages: []queue.ReceivedPhotoJob{{
			Message:       models.PhotoJobMessage{PhotoID: "photo-1"},
			ReceiptHandle: "receipt-1",
		}},
	}
	service := Service{
		Jobs:      jobs,
		Photos:    photos,
		Objects:   objects,
		TempFiles: &fakeTempFileStorageWorker{},
		Queue:     queue,
	}

	if err := service.ProcessQueueBatch(context.Background(), 1, 1); err == nil {
		t.Fatal("expected transient processing error")
	}
	if queue.deleteCalls != 0 {
		t.Fatalf("message should remain in queue for retry, got %+v", queue)
	}
}

func TestRunRetriesAfterQueueError(t *testing.T) {
	originalBackoff := errorBackoff
	originalTicker := publishTickerInterval
	errorBackoff = 5 * time.Millisecond
	publishTickerInterval = 20 * time.Millisecond
	defer func() {
		errorBackoff = originalBackoff
		publishTickerInterval = originalTicker
	}()

	ctx, cancel := context.WithCancel(context.Background())
	queue := &fakeQueue{
		receiveErrs: []error{errors.New("temporary sqs receive failure"), nil},
		receiveHook: func(call int) {
			if call >= 2 {
				cancel()
			}
		},
	}
	service := Service{
		Jobs:      &fakeJobStore{},
		Photos:    &fakePhotoStoreWorker{},
		Objects:   &fakeObjectStorageWorker{},
		TempFiles: &fakeTempFileStorageWorker{},
		Queue:     queue,
		Publish:   queue,
	}

	if err := service.Run(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation after retry, got %v", err)
	}
	if queue.receiveCalls < 2 {
		t.Fatalf("expected worker to retry receive after error, got %d calls", queue.receiveCalls)
	}
}

func TestRunRetriesAfterPublishError(t *testing.T) {
	originalBackoff := errorBackoff
	originalTicker := publishTickerInterval
	errorBackoff = 5 * time.Millisecond
	publishTickerInterval = 5 * time.Millisecond
	defer func() {
		errorBackoff = originalBackoff
		publishTickerInterval = originalTicker
	}()

	ctx, cancel := context.WithCancel(context.Background())
	jobs := &fakeJobStore{
		pendingJobs: []models.PhotoJob{{JobID: "job-1", PhotoID: "photo-1", Status: models.JobStatusPending}},
	}
	queue := &fakeQueue{
		publishErrs: []error{errors.New("temporary sqs publish failure"), nil},
		publishHook: func(call int) {
			if call >= 2 {
				cancel()
			}
		},
	}
	service := Service{
		Jobs:      jobs,
		Photos:    &fakePhotoStoreWorker{},
		Objects:   &fakeObjectStorageWorker{},
		TempFiles: &fakeTempFileStorageWorker{},
		Queue:     queue,
		Publish:   queue,
	}

	if err := service.Run(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation after retry, got %v", err)
	}
	if queue.publishCalls < 2 {
		t.Fatalf("expected worker to retry publish after error, got %d calls", queue.publishCalls)
	}
	if jobs.publishedCalls != 1 || jobs.publishedJobID != "job-1" {
		t.Fatalf("expected job to be marked published after retry, got %+v", jobs)
	}
}
