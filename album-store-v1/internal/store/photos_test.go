package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"album-store-v1/internal/models"

	"github.com/DATA-DOG/go-sqlmock"
)

func photoRowColumns() []string {
	return []string{"photo_id", "album_id", "seq", "status", "s3_key", "temp_path", "content_type", "url", "deleted"}
}

func TestCreatePhotoProcessingUsesAtomicAlbumSequenceAllocation(t *testing.T) {
	if strings.Contains(strings.ToUpper(allocatePhotoSeqSQL), "MAX(") {
		t.Fatal("photo sequence allocation must not derive seq from MAX(seq)")
	}

	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	mock.ExpectBegin()
	mock.ExpectQuery(allocatePhotoSeqSQL).
		WithArgs("album-1").
		WillReturnRows(sqlmock.NewRows([]string{"next_seq"}).AddRow(1))
	mock.ExpectQuery(insertPhotoSQL).
		WithArgs("photo-1", "album-1", 1, "uploads/photo-1.jpg", "/tmp/photo-1.jpg", "image/jpeg").
		WillReturnRows(sqlmock.NewRows(photoRowColumns()).
			AddRow("photo-1", "album-1", 1, models.PhotoStatusProcessing, "uploads/photo-1.jpg", "/tmp/photo-1.jpg", "image/jpeg", "", false))
	mock.ExpectCommit()

	photo, err := store.CreatePhotoProcessing(context.Background(), "album-1", "photo-1", "uploads/photo-1.jpg", "/tmp/photo-1.jpg", "image/jpeg")
	if err != nil {
		t.Fatalf("create photo processing: %v", err)
	}
	if photo.Seq != 1 || photo.TempPath != "/tmp/photo-1.jpg" || photo.ContentType != "image/jpeg" {
		t.Fatalf("unexpected created photo: %+v", photo)
	}
}

func TestCreatePhotoProcessingMissingAlbumRollsBackAndReturnsErrNotFound(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	mock.ExpectBegin()
	mock.ExpectQuery(allocatePhotoSeqSQL).
		WithArgs("missing-album").
		WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()

	if _, err := store.CreatePhotoProcessing(context.Background(), "missing-album", "photo-1", "uploads/photo-1.jpg", "/tmp/photo-1.jpg", "image/jpeg"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestCreatePhotoProcessingJobCreatesPhotoAndPendingJobInOneTransaction(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	mock.ExpectBegin()
	mock.ExpectQuery(allocatePhotoSeqSQL).
		WithArgs("album-1").
		WillReturnRows(sqlmock.NewRows([]string{"next_seq"}).AddRow(1))
	mock.ExpectQuery(insertPhotoSQL).
		WithArgs("photo-1", "album-1", 1, "uploads/photo-1.jpg", "/tmp/photo-1.jpg", "image/jpeg").
		WillReturnRows(sqlmock.NewRows(photoRowColumns()).
			AddRow("photo-1", "album-1", 1, models.PhotoStatusProcessing, "uploads/photo-1.jpg", "/tmp/photo-1.jpg", "image/jpeg", "", false))
	mock.ExpectExec(insertPhotoJobSQL).
		WithArgs("job-1", "photo-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	photo, err := store.CreatePhotoProcessingJob(context.Background(), "album-1", "photo-1", "job-1", "uploads/photo-1.jpg", "/tmp/photo-1.jpg", "image/jpeg")
	if err != nil {
		t.Fatalf("create photo processing job: %v", err)
	}
	if photo.PhotoID != "photo-1" || photo.TempPath != "/tmp/photo-1.jpg" {
		t.Fatalf("unexpected photo %+v", photo)
	}
}

func TestCreatePhotoProcessingJobRollsBackWhenJobInsertFails(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	mock.ExpectBegin()
	mock.ExpectQuery(allocatePhotoSeqSQL).
		WithArgs("album-1").
		WillReturnRows(sqlmock.NewRows([]string{"next_seq"}).AddRow(2))
	mock.ExpectQuery(insertPhotoSQL).
		WithArgs("photo-1", "album-1", 2, "uploads/photo-1.jpg", "/tmp/photo-1.jpg", "image/jpeg").
		WillReturnRows(sqlmock.NewRows(photoRowColumns()).
			AddRow("photo-1", "album-1", 2, models.PhotoStatusProcessing, "uploads/photo-1.jpg", "/tmp/photo-1.jpg", "image/jpeg", "", false))
	mock.ExpectExec(insertPhotoJobSQL).
		WithArgs("job-1", "photo-1").
		WillReturnError(errors.New("job insert failed"))
	mock.ExpectRollback()

	if _, err := store.CreatePhotoProcessingJob(context.Background(), "album-1", "photo-1", "job-1", "uploads/photo-1.jpg", "/tmp/photo-1.jpg", "image/jpeg"); err == nil {
		t.Fatal("expected job insert failure")
	}
}

func TestGetPhotoReturnsPhoto(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	mock.ExpectQuery(getPhotoSQL).
		WithArgs("album-1", "photo-1").
		WillReturnRows(sqlmock.NewRows(photoRowColumns()).
			AddRow("photo-1", "album-1", 2, models.PhotoStatusCompleted, "uploads/photo-1.jpg", "", "", "https://bucket/photo-1.jpg", false))

	photo, err := store.GetPhoto(context.Background(), "album-1", "photo-1")
	if err != nil {
		t.Fatalf("get photo: %v", err)
	}
	if photo.URL != "https://bucket/photo-1.jpg" {
		t.Fatalf("expected completed photo URL, got %+v", photo)
	}
}

func TestGetPhotoByIDReturnsProcessingPhoto(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	mock.ExpectQuery(getPhotoByIDSQL).
		WithArgs("photo-1").
		WillReturnRows(sqlmock.NewRows(photoRowColumns()).
			AddRow("photo-1", "album-1", 2, models.PhotoStatusProcessing, "uploads/photo-1.jpg", "/tmp/photo-1.jpg", "image/jpeg", "", false))

	photo, err := store.GetPhotoByID(context.Background(), "photo-1")
	if err != nil {
		t.Fatalf("get photo by id: %v", err)
	}
	if photo.TempPath != "/tmp/photo-1.jpg" || photo.ContentType != "image/jpeg" {
		t.Fatalf("unexpected photo %+v", photo)
	}
}

func TestListRecoverablePhotosReturnsProcessingRowsWithTempPaths(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	mock.ExpectQuery(listRecoverablePhotosSQL).
		WillReturnRows(sqlmock.NewRows([]string{"photo_id"}).
			AddRow("photo-1").
			AddRow("photo-2"))

	photoIDs, err := store.ListRecoverablePhotos(context.Background())
	if err != nil {
		t.Fatalf("list recoverable photos: %v", err)
	}
	if len(photoIDs) != 2 || photoIDs[0] != "photo-1" || photoIDs[1] != "photo-2" {
		t.Fatalf("unexpected recoverable photo ids %+v", photoIDs)
	}
}

func TestMarkPhotoCompletedReturnsErrNotFoundWhenNoRowIsUpdated(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	mock.ExpectExec(markPhotoCompletedSQL).
		WithArgs("album-1", "photo-1", "https://bucket/photo-1.jpg").
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := store.MarkPhotoCompleted(context.Background(), "album-1", "photo-1", "https://bucket/photo-1.jpg"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestMarkPhotoFailedReturnsErrNotFoundWhenNoRowIsUpdated(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	mock.ExpectExec(markPhotoFailedSQL).
		WithArgs("album-1", "photo-1").
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := store.MarkPhotoFailed(context.Background(), "album-1", "photo-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestMarkPhotoDeletedReturnsPhotoForCleanup(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	mock.ExpectQuery(markPhotoDeletedSQL).
		WithArgs("album-1", "photo-1").
		WillReturnRows(sqlmock.NewRows(photoRowColumns()).
			AddRow("photo-1", "album-1", 2, models.PhotoStatusCompleted, "uploads/photo-1.jpg", "/tmp/photo-1.jpg", "image/jpeg", "https://bucket/photo-1.jpg", true))

	photo, err := store.MarkPhotoDeleted(context.Background(), "album-1", "photo-1")
	if err != nil {
		t.Fatalf("mark photo deleted: %v", err)
	}
	if !photo.Deleted || photo.TempPath != "/tmp/photo-1.jpg" {
		t.Fatalf("unexpected deleted photo payload: %+v", photo)
	}
}

func TestMarkPhotoJobPublishedReturnsErrNotFoundWhenJobCannotBeUpdated(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	mock.ExpectExec(markPhotoJobPublishedSQL).
		WithArgs("job-1").
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := store.MarkPhotoJobPublished(context.Background(), "job-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestListPendingPhotoJobsReturnsEveryPendingRowUpToLimit(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	mock.ExpectQuery(listPendingPhotoJobsSQL).
		WithArgs(10).
		WillReturnRows(sqlmock.NewRows([]string{"job_id", "photo_id", "status", "attempts"}).
			AddRow("job-1", "photo-1", models.JobStatusPending, 0).
			AddRow("job-2", "photo-2", models.JobStatusPending, 1))

	jobs, err := store.ListPendingPhotoJobs(context.Background(), 10)
	if err != nil {
		t.Fatalf("list pending jobs: %v", err)
	}
	if len(jobs) != 2 || jobs[0].JobID != "job-1" || jobs[1].JobID != "job-2" {
		t.Fatalf("unexpected jobs %+v", jobs)
	}
}

func TestMarkPhotoJobCompletedByPhotoIDReturnsErrNotFoundWhenNoRowUpdated(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	mock.ExpectExec(markPhotoJobCompletedByPhotoIDSQL).
		WithArgs("photo-1").
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := store.MarkPhotoJobCompletedByPhotoID(context.Background(), "photo-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestMarkPhotoJobFailedByPhotoIDReturnsErrNotFoundWhenNoRowUpdated(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	mock.ExpectExec(markPhotoJobFailedByPhotoIDSQL).
		WithArgs("photo-1").
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := store.MarkPhotoJobFailedByPhotoID(context.Background(), "photo-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
