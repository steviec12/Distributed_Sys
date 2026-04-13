package pipeline

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"album-store-v1/internal/models"
	"album-store-v1/internal/store"
)

type fakePhotoStore struct {
	photo          models.Photo
	getErr         error
	completedAlbum string
	completedPhoto string
	completedURL   string
	completedCalls int
	completedErr   error
	failedAlbum    string
	failedPhoto    string
	failedCalls    int
	failedErr      error
	recoverableIDs []string
	recoverableErr error
}

func (f *fakePhotoStore) GetPhotoByID(_ context.Context, photoID string) (models.Photo, error) {
	if f.getErr != nil {
		return models.Photo{}, f.getErr
	}
	if f.photo.PhotoID == "" {
		f.photo.PhotoID = photoID
	}
	return f.photo, nil
}

func (f *fakePhotoStore) MarkPhotoCompleted(_ context.Context, albumID, photoID, url string) error {
	f.completedCalls++
	f.completedAlbum = albumID
	f.completedPhoto = photoID
	f.completedURL = url
	return f.completedErr
}

func (f *fakePhotoStore) MarkPhotoFailed(_ context.Context, albumID, photoID string) error {
	f.failedCalls++
	f.failedAlbum = albumID
	f.failedPhoto = photoID
	return f.failedErr
}

func (f *fakePhotoStore) ListRecoverablePhotos(_ context.Context) ([]string, error) {
	if f.recoverableErr != nil {
		return nil, f.recoverableErr
	}
	return f.recoverableIDs, nil
}

type fakeObjectStorage struct {
	putKey         string
	putContentType string
	putCalls       int
	putErr         error
	deleteKey      string
	deleteCalls    int
	deleteErr      error
}

func (f *fakeObjectStorage) PutObject(_ context.Context, key string, body io.Reader, _ int64, contentType string) error {
	f.putCalls++
	f.putKey = key
	f.putContentType = contentType
	if body == nil {
		return errors.New("missing body")
	}
	_, _ = io.ReadAll(body)
	return f.putErr
}

func (f *fakeObjectStorage) DeleteObject(_ context.Context, key string) error {
	f.deleteCalls++
	f.deleteKey = key
	return f.deleteErr
}

func (f *fakeObjectStorage) PublicURL(key string) string {
	return "https://bucket/" + key
}

type fakeTempFiles struct {
	openPath    string
	openCalls   int
	openErr     error
	deletePath  string
	deleteCalls int
	deleteErr   error
}

func (f *fakeTempFiles) Open(path string) (io.ReadCloser, error) {
	f.openCalls++
	f.openPath = path
	if f.openErr != nil {
		return nil, f.openErr
	}
	return io.NopCloser(strings.NewReader("file-bytes")), nil
}

func (f *fakeTempFiles) Delete(path string) error {
	f.deleteCalls++
	f.deletePath = path
	return f.deleteErr
}

func TestProcessPhotoUploadsCompletesAndDeletesTempFile(t *testing.T) {
	photos := &fakePhotoStore{
		photo: models.Photo{
			PhotoID:     "photo-1",
			AlbumID:     "album-1",
			Status:      models.PhotoStatusProcessing,
			S3Key:       "albums/album-1/photos/photo-1.jpg",
			TempPath:    "/tmp/photo-1.jpg",
			ContentType: "image/jpeg",
		},
	}
	objects := &fakeObjectStorage{}
	tempFiles := &fakeTempFiles{}
	p := New(photos, objects, tempFiles, 8)

	if err := p.processPhoto(context.Background(), "photo-1"); err != nil {
		t.Fatalf("process photo: %v", err)
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
}

func TestProcessPhotoMarksMissingTempPathFailed(t *testing.T) {
	photos := &fakePhotoStore{
		photo: models.Photo{
			PhotoID: "photo-1",
			AlbumID: "album-1",
			Status:  models.PhotoStatusProcessing,
			S3Key:   "albums/album-1/photos/photo-1.jpg",
		},
	}
	p := New(photos, &fakeObjectStorage{}, &fakeTempFiles{}, 8)

	if err := p.processPhoto(context.Background(), "photo-1"); err != nil {
		t.Fatalf("process photo: %v", err)
	}
	if photos.failedCalls != 1 || photos.failedAlbum != "album-1" || photos.failedPhoto != "photo-1" {
		t.Fatalf("expected mark failed, got %+v", photos)
	}
}

func TestProcessPhotoCleansUpUploadedObjectWhenPhotoWasDeleted(t *testing.T) {
	photos := &fakePhotoStore{
		photo: models.Photo{
			PhotoID:     "photo-1",
			AlbumID:     "album-1",
			Status:      models.PhotoStatusProcessing,
			S3Key:       "albums/album-1/photos/photo-1.jpg",
			TempPath:    "/tmp/photo-1.jpg",
			ContentType: "image/jpeg",
		},
		completedErr: store.ErrNotFound,
	}
	objects := &fakeObjectStorage{}
	tempFiles := &fakeTempFiles{}
	p := New(photos, objects, tempFiles, 8)

	if err := p.processPhoto(context.Background(), "photo-1"); err != nil {
		t.Fatalf("process photo: %v", err)
	}
	if objects.deleteCalls != 1 || objects.deleteKey != "albums/album-1/photos/photo-1.jpg" {
		t.Fatalf("expected uploaded object cleanup, got %+v", objects)
	}
	if tempFiles.deleteCalls != 1 || tempFiles.deletePath != "/tmp/photo-1.jpg" {
		t.Fatalf("expected temp cleanup, got %+v", tempFiles)
	}
}
