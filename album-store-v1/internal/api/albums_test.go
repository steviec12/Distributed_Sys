package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"album-store-v1/internal/models"
	"album-store-v1/internal/store"
)

type fakeAlbumStore struct {
	putInput   models.Album
	putResult  models.Album
	putErr     error
	putCalls   int
	getAlbumID string
	getResult  models.Album
	getErr     error
	listResult []models.Album
	listErr    error
	listCalls  int
	getCalls   int
}

func (f *fakeAlbumStore) PutAlbum(_ context.Context, album models.Album) (models.Album, error) {
	f.putCalls++
	f.putInput = album
	if f.putErr != nil {
		return models.Album{}, f.putErr
	}
	return f.putResult, nil
}

func (f *fakeAlbumStore) GetAlbum(_ context.Context, albumID string) (models.Album, error) {
	f.getCalls++
	f.getAlbumID = albumID
	if f.getErr != nil {
		return models.Album{}, f.getErr
	}
	return f.getResult, nil
}

func (f *fakeAlbumStore) ListAlbums(_ context.Context) ([]models.Album, error) {
	f.listCalls++
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.listResult, nil
}

func TestPutAlbumReturnsSavedAlbum(t *testing.T) {
	album := models.Album{
		AlbumID:     "album-1",
		Title:       "Summer",
		Description: "Trip photos",
		Owner:       "student@northeastern.edu",
	}
	fake := &fakeAlbumStore{putResult: album}
	router := NewRouter(Dependencies{Albums: fake})

	req := httptest.NewRequest(http.MethodPut, "/albums/album-1", strings.NewReader(`{
		"album_id": "album-1",
		"title": "Summer",
		"description": "Trip photos",
		"owner": "student@northeastern.edu"
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d with body %s", rec.Code, rec.Body.String())
	}
	if fake.putCalls != 1 {
		t.Fatalf("expected PutAlbum to be called once, got %d", fake.putCalls)
	}
	if fake.putInput != album {
		t.Fatalf("expected PutAlbum input %+v, got %+v", album, fake.putInput)
	}
	if fake.getCalls != 1 {
		t.Fatalf("expected GetAlbum to be called once, got %d", fake.getCalls)
	}
	if got := rec.Body.String(); got != `{"album_id":"album-1","title":"Summer","description":"Trip photos","owner":"student@northeastern.edu"}` {
		t.Fatalf("unexpected body %s", got)
	}
}

func TestPutAlbumRejectsPathBodyMismatch(t *testing.T) {
	fake := &fakeAlbumStore{}
	router := NewRouter(Dependencies{Albums: fake})

	req := httptest.NewRequest(http.MethodPut, "/albums/path-id", strings.NewReader(`{
		"album_id": "body-id",
		"title": "Summer",
		"description": "Trip photos",
		"owner": "student@northeastern.edu"
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
	if fake.putCalls != 0 {
		t.Fatalf("expected store not to be called on mismatch, got %d calls", fake.putCalls)
	}
}

func TestPutAlbumAcceptsMissingOptionalFieldsForNewAlbum(t *testing.T) {
	fake := &fakeAlbumStore{
		getErr: store.ErrNotFound,
		putResult: models.Album{
			AlbumID:     "album-1",
			Title:       "Summer",
			Description: "",
			Owner:       "student@northeastern.edu",
		},
	}
	router := NewRouter(Dependencies{Albums: fake})

	req := httptest.NewRequest(http.MethodPut, "/albums/album-1", strings.NewReader(`{
		"title": "Summer",
		"owner": "student@northeastern.edu"
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d with body %s", rec.Code, rec.Body.String())
	}
	if fake.putCalls != 1 {
		t.Fatalf("expected store to be called once, got %d calls", fake.putCalls)
	}
	expected := models.Album{
		AlbumID:     "album-1",
		Title:       "Summer",
		Description: "",
		Owner:       "student@northeastern.edu",
	}
	if fake.putInput != expected {
		t.Fatalf("expected PutAlbum input %+v, got %+v", expected, fake.putInput)
	}
}

func TestPutAlbumSparseUpdatePreservesExistingFields(t *testing.T) {
	existing := models.Album{
		AlbumID:     "album-1",
		Title:       "Summer",
		Description: "Trip photos",
		Owner:       "student@northeastern.edu",
	}
	fake := &fakeAlbumStore{
		getResult: existing,
		putResult: models.Album{
			AlbumID:     "album-1",
			Title:       "Updated",
			Description: "Trip photos",
			Owner:       "student@northeastern.edu",
		},
	}
	router := NewRouter(Dependencies{Albums: fake})

	req := httptest.NewRequest(http.MethodPut, "/albums/album-1", strings.NewReader(`{
		"title": "Updated"
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d with body %s", rec.Code, rec.Body.String())
	}
	expected := models.Album{
		AlbumID:     "album-1",
		Title:       "Updated",
		Description: "Trip photos",
		Owner:       "student@northeastern.edu",
	}
	if fake.putInput != expected {
		t.Fatalf("expected PutAlbum input %+v, got %+v", expected, fake.putInput)
	}
}

func TestPutAlbumRejectsMalformedJSON(t *testing.T) {
	fake := &fakeAlbumStore{}
	router := NewRouter(Dependencies{Albums: fake})

	req := httptest.NewRequest(http.MethodPut, "/albums/album-1", strings.NewReader(`{`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
	if fake.putCalls != 0 {
		t.Fatalf("expected store not to be called for malformed JSON, got %d calls", fake.putCalls)
	}
}

func TestGetAlbumReturnsAlbum(t *testing.T) {
	album := models.Album{
		AlbumID:     "album-1",
		Title:       "Summer",
		Description: "Trip photos",
		Owner:       "student@northeastern.edu",
	}
	fake := &fakeAlbumStore{getResult: album}
	router := NewRouter(Dependencies{Albums: fake})

	req := httptest.NewRequest(http.MethodGet, "/albums/album-1", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if fake.getAlbumID != "album-1" {
		t.Fatalf("expected lookup album-1, got %q", fake.getAlbumID)
	}
	if got := rec.Body.String(); got != `{"album_id":"album-1","title":"Summer","description":"Trip photos","owner":"student@northeastern.edu"}` {
		t.Fatalf("unexpected body %s", got)
	}
}

func TestGetAlbumMissingReturns404(t *testing.T) {
	fake := &fakeAlbumStore{getErr: store.ErrNotFound}
	router := NewRouter(Dependencies{Albums: fake})

	req := httptest.NewRequest(http.MethodGet, "/albums/missing", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rec.Code)
	}
	if got := rec.Body.String(); got != `{"error":"not found"}` {
		t.Fatalf("unexpected body %s", got)
	}
}

func TestListAlbumsReturnsBareArray(t *testing.T) {
	fake := &fakeAlbumStore{listResult: []models.Album{
		{AlbumID: "album-1", Title: "A", Description: "first", Owner: "a@example.com"},
		{AlbumID: "album-2", Title: "B", Description: "second", Owner: "b@example.com"},
	}}
	router := NewRouter(Dependencies{Albums: fake})

	req := httptest.NewRequest(http.MethodGet, "/albums", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if got := rec.Body.String(); got != `[{"album_id":"album-1","title":"A","description":"first","owner":"a@example.com"},{"album_id":"album-2","title":"B","description":"second","owner":"b@example.com"}]` {
		t.Fatalf("unexpected body %s", got)
	}
}

func TestListAlbumsReturnsEmptyArrayNotNull(t *testing.T) {
	fake := &fakeAlbumStore{}
	router := NewRouter(Dependencies{Albums: fake})

	req := httptest.NewRequest(http.MethodGet, "/albums", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if got := rec.Body.String(); got != `[]` {
		t.Fatalf("expected empty array, got %s", got)
	}
}

func TestAlbumStoreErrorsReturn500(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		body   string
		fake   *fakeAlbumStore
	}{
		{
			name:   "put",
			method: http.MethodPut,
			path:   "/albums/album-1",
			body:   `{"album_id":"album-1","title":"A","description":"B","owner":"C"}`,
			fake:   &fakeAlbumStore{putErr: errors.New("db down")},
		},
		{
			name:   "get",
			method: http.MethodGet,
			path:   "/albums/album-1",
			fake:   &fakeAlbumStore{getErr: errors.New("db down")},
		},
		{
			name:   "list",
			method: http.MethodGet,
			path:   "/albums",
			fake:   &fakeAlbumStore{listErr: errors.New("db down")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := NewRouter(Dependencies{Albums: tt.fake})
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("expected status 500, got %d", rec.Code)
			}
		})
	}
}
