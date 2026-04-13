package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"album-store-v1/internal/models"
	"album-store-v1/internal/store"
)

type fakePhotoStore struct {
	getAlbumID       string
	getPhotoID       string
	getResult        models.Photo
	getErr           error
	getCalls         int
	getDeleteAlbumID string
	getDeletePhotoID string
	getDeleteResult  models.Photo
	getDeleteErr     error
	getDeleteCalls   int

	createAlbumID     string
	createPhotoID     string
	createS3Key       string
	createTempPath    string
	createContentType string
	createResult      models.Photo
	createErr         error
	createCalls       int

	markFailedAlbum string
	markFailedPhoto string
	markFailedCalls int
	markFailedErr   error

	markDeletedAlbum  string
	markDeletedPhoto  string
	markDeletedResult models.Photo
	markDeletedErr    error
	markDeletedCalls  int
}

func (f *fakePhotoStore) GetPhoto(_ context.Context, albumID, photoID string) (models.Photo, error) {
	f.getCalls++
	f.getAlbumID = albumID
	f.getPhotoID = photoID
	if f.getErr != nil {
		return models.Photo{}, f.getErr
	}
	return f.getResult, nil
}

func (f *fakePhotoStore) GetPhotoForDelete(_ context.Context, albumID, photoID string) (models.Photo, error) {
	f.getDeleteCalls++
	f.getDeleteAlbumID = albumID
	f.getDeletePhotoID = photoID
	if f.getDeleteErr != nil {
		return models.Photo{}, f.getDeleteErr
	}
	return f.getDeleteResult, nil
}

func (f *fakePhotoStore) CreatePhotoProcessing(_ context.Context, albumID, photoID, s3Key, tempPath, contentType string) (models.Photo, error) {
	f.createCalls++
	f.createAlbumID = albumID
	f.createPhotoID = photoID
	f.createS3Key = s3Key
	f.createTempPath = tempPath
	f.createContentType = contentType
	if f.createErr != nil {
		return models.Photo{}, f.createErr
	}
	return f.createResult, nil
}

func (f *fakePhotoStore) MarkPhotoFailed(_ context.Context, albumID, photoID string) error {
	f.markFailedCalls++
	f.markFailedAlbum = albumID
	f.markFailedPhoto = photoID
	return f.markFailedErr
}

func (f *fakePhotoStore) MarkPhotoDeleted(_ context.Context, albumID, photoID string) (models.Photo, error) {
	f.markDeletedCalls++
	f.markDeletedAlbum = albumID
	f.markDeletedPhoto = photoID
	if f.markDeletedErr != nil {
		return models.Photo{}, f.markDeletedErr
	}
	return f.markDeletedResult, nil
}

type fakeObjectStorage struct {
	deleteKey   string
	deleteCalls int
	deleteErr   error
}

func (f *fakeObjectStorage) DeleteObject(_ context.Context, key string) error {
	f.deleteCalls++
	f.deleteKey = key
	return f.deleteErr
}

func (f *fakeObjectStorage) PublicURL(key string) string {
	return "https://bucket/" + key
}

type fakeTempFileStorage struct {
	saveFilename string
	saveBodySeen bool
	savePath     string
	saveErr      error
	deletePath   string
	deleteCalls  int
	deleteErr    error
}

func (f *fakeTempFileStorage) Save(body io.Reader, filename string) (string, error) {
	f.saveFilename = filename
	if body != nil {
		f.saveBodySeen = true
	}
	if f.saveErr != nil {
		return "", f.saveErr
	}
	if f.savePath != "" {
		return f.savePath, nil
	}
	return "/tmp/fake-photo.jpg", nil
}

func (f *fakeTempFileStorage) Delete(path string) error {
	f.deleteCalls++
	f.deletePath = path
	return f.deleteErr
}

type fakePipeline struct {
	submitPhotoID string
	submitCalls   int
	submitErr     error
}

func (f *fakePipeline) Submit(photoID string) error {
	f.submitCalls++
	f.submitPhotoID = photoID
	return f.submitErr
}

func fixedIDs(ids ...string) func() string {
	index := 0
	return func() string {
		if index >= len(ids) {
			return "extra-id"
		}
		value := ids[index]
		index++
		return value
	}
}

func multipartBody() *strings.Reader {
	return strings.NewReader("--boundary\r\nContent-Disposition: form-data; name=\"photo\"; filename=\"summer.jpg\"\r\nContent-Type: image/jpeg\r\n\r\nfile-bytes\r\n--boundary--\r\n")
}

func TestUploadPhotoReturnsAcceptedResponse(t *testing.T) {
	fakeStore := &fakePhotoStore{
		createResult: models.Photo{
			PhotoID: "photo-1",
			AlbumID: "album-1",
			Seq:     4,
			Status:  models.PhotoStatusProcessing,
			S3Key:   "albums/album-1/photos/photo-1.jpg",
		},
	}
	fakeTemps := &fakeTempFileStorage{savePath: "/tmp/photo-1.jpg"}
	fakeJobs := &fakePipeline{}
	router := NewRouter(Dependencies{
		Photos:    fakeStore,
		Objects:   &fakeObjectStorage{},
		TempFiles: fakeTemps,
		Pipeline:  fakeJobs,
		NewID:     fixedIDs("photo-1"),
	})

	req := httptest.NewRequest(http.MethodPost, "/albums/album-1/photos", multipartBody())
	req.Header.Set("Content-Type", "multipart/form-data; boundary=boundary")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d with body %s", rec.Code, rec.Body.String())
	}
	if !fakeTemps.saveBodySeen || fakeTemps.saveFilename != "summer.jpg" {
		t.Fatalf("unexpected temp file save state %+v", fakeTemps)
	}
	if fakeStore.createCalls != 1 {
		t.Fatalf("expected one create call, got %+v", fakeStore)
	}
	if fakeStore.createAlbumID != "album-1" || fakeStore.createPhotoID != "photo-1" {
		t.Fatalf("unexpected create identifiers %+v", fakeStore)
	}
	if fakeStore.createTempPath != "/tmp/photo-1.jpg" || fakeStore.createContentType != "image/jpeg" {
		t.Fatalf("unexpected staged upload state %+v", fakeStore)
	}
	if fakeJobs.submitCalls != 1 || fakeJobs.submitPhotoID != "photo-1" {
		t.Fatalf("unexpected pipeline submit %+v", fakeJobs)
	}
	if got := rec.Body.String(); got != `{"photo_id":"photo-1","seq":4,"status":"processing"}` {
		t.Fatalf("unexpected body %s", got)
	}
}

func TestUploadPhotoMissingFieldReturns400(t *testing.T) {
	router := NewRouter(Dependencies{
		Photos:    &fakePhotoStore{},
		Objects:   &fakeObjectStorage{},
		TempFiles: &fakeTempFileStorage{},
		Pipeline:  &fakePipeline{},
	})

	req := httptest.NewRequest(http.MethodPost, "/albums/album-1/photos", strings.NewReader(""))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=boundary")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
	if got := rec.Body.String(); got != `{"error":"missing photo field"}` {
		t.Fatalf("unexpected body %s", got)
	}
}

func TestUploadPhotoMissingAlbumReturns404AndDeletesTempFile(t *testing.T) {
	fakeStore := &fakePhotoStore{createErr: store.ErrNotFound}
	fakeTemps := &fakeTempFileStorage{savePath: "/tmp/photo-1.jpg"}
	router := NewRouter(Dependencies{
		Photos:    fakeStore,
		Objects:   &fakeObjectStorage{},
		TempFiles: fakeTemps,
		Pipeline:  &fakePipeline{},
		NewID:     fixedIDs("photo-1"),
	})

	req := httptest.NewRequest(http.MethodPost, "/albums/missing/photos", multipartBody())
	req.Header.Set("Content-Type", "multipart/form-data; boundary=boundary")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d with body %s", rec.Code, rec.Body.String())
	}
	if fakeTemps.deleteCalls != 1 || fakeTemps.deletePath != "/tmp/photo-1.jpg" {
		t.Fatalf("expected temp cleanup, got %+v", fakeTemps)
	}
}

func TestUploadPhotoSubmitFailureReturns500MarksFailedAndDeletesTempFile(t *testing.T) {
	fakeStore := &fakePhotoStore{
		createResult: models.Photo{
			PhotoID: "photo-1",
			AlbumID: "album-1",
			Seq:     1,
			Status:  models.PhotoStatusProcessing,
			S3Key:   "albums/album-1/photos/photo-1.jpg",
		},
	}
	fakeTemps := &fakeTempFileStorage{savePath: "/tmp/photo-1.jpg"}
	fakeJobs := &fakePipeline{submitErr: errors.New("queue full")}
	router := NewRouter(Dependencies{
		Photos:    fakeStore,
		Objects:   &fakeObjectStorage{},
		TempFiles: fakeTemps,
		Pipeline:  fakeJobs,
		NewID:     fixedIDs("photo-1"),
	})

	req := httptest.NewRequest(http.MethodPost, "/albums/album-1/photos", multipartBody())
	req.Header.Set("Content-Type", "multipart/form-data; boundary=boundary")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d with body %s", rec.Code, rec.Body.String())
	}
	if fakeStore.markFailedCalls != 1 || fakeStore.markFailedAlbum != "album-1" || fakeStore.markFailedPhoto != "photo-1" {
		t.Fatalf("expected mark failed after submit failure, got %+v", fakeStore)
	}
	if fakeTemps.deleteCalls != 1 || fakeTemps.deletePath != "/tmp/photo-1.jpg" {
		t.Fatalf("expected temp cleanup after submit failure, got %+v", fakeTemps)
	}
}

func TestGetPhotoProcessingReturnsStatusWithoutURL(t *testing.T) {
	fake := &fakePhotoStore{
		getResult: models.Photo{
			PhotoID: "photo-1",
			AlbumID: "album-1",
			Seq:     4,
			Status:  models.PhotoStatusProcessing,
			S3Key:   "uploads/photo-1.jpg",
		},
	}
	router := NewRouter(Dependencies{Photos: fake})

	req := httptest.NewRequest(http.MethodGet, "/albums/album-1/photos/photo-1", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if got := rec.Body.String(); got != `{"photo_id":"photo-1","album_id":"album-1","seq":4,"status":"processing"}` {
		t.Fatalf("unexpected body %s", got)
	}
}

func TestDeletePhotoReturns204AndDeletesTempAndObject(t *testing.T) {
	fakeStore := &fakePhotoStore{
		getDeleteResult: models.Photo{
			PhotoID:  "photo-1",
			AlbumID:  "album-1",
			S3Key:    "albums/album-1/photos/photo-1.jpg",
			TempPath: "/tmp/photo-1.jpg",
			Deleted:  false,
		},
		markDeletedResult: models.Photo{
			PhotoID:  "photo-1",
			AlbumID:  "album-1",
			S3Key:    "albums/album-1/photos/photo-1.jpg",
			TempPath: "/tmp/photo-1.jpg",
			Deleted:  true,
		},
	}
	fakeObjects := &fakeObjectStorage{}
	fakeTemps := &fakeTempFileStorage{}
	router := NewRouter(Dependencies{Photos: fakeStore, Objects: fakeObjects, TempFiles: fakeTemps})

	req := httptest.NewRequest(http.MethodDelete, "/albums/album-1/photos/photo-1", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d", rec.Code)
	}
	if fakeTemps.deleteCalls != 1 || fakeTemps.deletePath != "/tmp/photo-1.jpg" {
		t.Fatalf("expected temp cleanup %+v", fakeTemps)
	}
	if fakeObjects.deleteCalls != 1 || fakeObjects.deleteKey != "albums/album-1/photos/photo-1.jpg" {
		t.Fatalf("expected object delete %+v", fakeObjects)
	}
}
