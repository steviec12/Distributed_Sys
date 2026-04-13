package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"album-store-v1/internal/models"
)

// putAlbumSQL uses Postgres upsert so concurrent PUTs for one album_id collapse
// into a single row instead of creating duplicates.
const putAlbumSQL = `
INSERT INTO albums (album_id, title, description, owner, next_seq)
VALUES ($1, $2, $3, $4, 1)
ON CONFLICT (album_id) DO UPDATE SET
    title = EXCLUDED.title,
    description = EXCLUDED.description,
    owner = EXCLUDED.owner
RETURNING album_id, title, description, owner`

const getAlbumSQL = `
SELECT album_id, title, description, owner
FROM albums
WHERE album_id = $1`

const listAlbumsSQL = `
SELECT album_id, title, description, owner
FROM albums
ORDER BY album_id`

// PutAlbum creates or updates one album without resetting its next_seq counter.
func (s *Store) PutAlbum(ctx context.Context, album models.Album) (models.Album, error) {
	var saved models.Album
	err := s.db.QueryRowContext(
		ctx,
		putAlbumSQL,
		album.AlbumID,
		album.Title,
		album.Description,
		album.Owner,
	).Scan(&saved.AlbumID, &saved.Title, &saved.Description, &saved.Owner)
	if err != nil {
		return models.Album{}, fmt.Errorf("put album: %w", err)
	}

	return saved, nil
}

// GetAlbum loads one album by its public album_id.
func (s *Store) GetAlbum(ctx context.Context, albumID string) (models.Album, error) {
	var album models.Album
	err := s.db.QueryRowContext(ctx, getAlbumSQL, albumID).
		Scan(&album.AlbumID, &album.Title, &album.Description, &album.Owner)
	if errors.Is(err, sql.ErrNoRows) {
		return models.Album{}, ErrNotFound
	}
	if err != nil {
		return models.Album{}, fmt.Errorf("get album: %w", err)
	}

	return album, nil
}

// ListAlbums returns every album row without imposing an application limit.
func (s *Store) ListAlbums(ctx context.Context) ([]models.Album, error) {
	rows, err := s.db.QueryContext(ctx, listAlbumsSQL)
	if err != nil {
		return nil, fmt.Errorf("list albums: %w", err)
	}
	defer rows.Close()

	var albums []models.Album
	for rows.Next() {
		var album models.Album
		if err := rows.Scan(&album.AlbumID, &album.Title, &album.Description, &album.Owner); err != nil {
			return nil, fmt.Errorf("scan album: %w", err)
		}
		albums = append(albums, album)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate albums: %w", err)
	}

	return albums, nil
}
