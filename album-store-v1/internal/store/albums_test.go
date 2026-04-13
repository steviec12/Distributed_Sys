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

func newMockStore(t *testing.T) (*Store, sqlmock.Sqlmock, func()) {
	t.Helper()

	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}

	cleanup := func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sql expectations: %v", err)
		}
		db.Close()
	}

	return New(db), mock, cleanup
}

func TestPutAlbumUsesUpsertAndDoesNotResetSequenceOnUpdate(t *testing.T) {
	if !strings.Contains(putAlbumSQL, "ON CONFLICT (album_id) DO UPDATE") {
		t.Fatal("put album SQL must use a database-level upsert for idempotency")
	}
	if strings.Contains(putAlbumSQL, "next_seq =") {
		t.Fatal("album updates must not reset next_seq")
	}

	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	album := models.Album{
		AlbumID:     "album-1",
		Title:       "title",
		Description: "description",
		Owner:       "owner@example.com",
	}

	mock.ExpectQuery(putAlbumSQL).
		WithArgs(album.AlbumID, album.Title, album.Description, album.Owner).
		WillReturnRows(sqlmock.NewRows([]string{"album_id", "title", "description", "owner"}).
			AddRow(album.AlbumID, album.Title, album.Description, album.Owner))

	got, err := store.PutAlbum(context.Background(), album)
	if err != nil {
		t.Fatalf("put album: %v", err)
	}
	if got != album {
		t.Fatalf("expected saved album %+v, got %+v", album, got)
	}
}

func TestGetAlbumReturnsAlbum(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	expected := models.Album{
		AlbumID:     "album-1",
		Title:       "title",
		Description: "description",
		Owner:       "owner@example.com",
	}

	mock.ExpectQuery(getAlbumSQL).
		WithArgs(expected.AlbumID).
		WillReturnRows(sqlmock.NewRows([]string{"album_id", "title", "description", "owner"}).
			AddRow(expected.AlbumID, expected.Title, expected.Description, expected.Owner))

	got, err := store.GetAlbum(context.Background(), expected.AlbumID)
	if err != nil {
		t.Fatalf("get album: %v", err)
	}
	if got != expected {
		t.Fatalf("expected album %+v, got %+v", expected, got)
	}
}

func TestGetAlbumMissingReturnsErrNotFound(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	mock.ExpectQuery(getAlbumSQL).
		WithArgs("missing").
		WillReturnError(sql.ErrNoRows)

	if _, err := store.GetAlbum(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestListAlbumsReturnsEveryRowWithoutHardcodedLimit(t *testing.T) {
	if strings.Contains(strings.ToUpper(listAlbumsSQL), "LIMIT") {
		t.Fatal("list albums SQL must not use a hardcoded LIMIT")
	}

	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	mock.ExpectQuery(listAlbumsSQL).
		WillReturnRows(sqlmock.NewRows([]string{"album_id", "title", "description", "owner"}).
			AddRow("album-1", "title 1", "description 1", "owner1@example.com").
			AddRow("album-2", "title 2", "description 2", "owner2@example.com"))

	albums, err := store.ListAlbums(context.Background())
	if err != nil {
		t.Fatalf("list albums: %v", err)
	}
	if len(albums) != 2 {
		t.Fatalf("expected 2 albums, got %d", len(albums))
	}
	if albums[0].AlbumID != "album-1" || albums[1].AlbumID != "album-2" {
		t.Fatalf("expected both albums in result, got %+v", albums)
	}
}

func TestListAlbumsReturnsScanError(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	mock.ExpectQuery(listAlbumsSQL).
		WillReturnRows(sqlmock.NewRows([]string{"album_id", "title", "description", "owner"}).
			AddRow("album-1", "title", "description", nil))

	if _, err := store.ListAlbums(context.Background()); err == nil {
		t.Fatal("expected scan error")
	}
}
