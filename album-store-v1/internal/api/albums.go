package api

import (
	"context"
	"errors"
	"net/http"

	"album-store-v1/internal/models"
	"album-store-v1/internal/store"

	"github.com/gin-gonic/gin"
)

// AlbumStore captures the album operations the HTTP layer needs from storage.
type AlbumStore interface {
	PutAlbum(ctx context.Context, album models.Album) (models.Album, error)
	GetAlbum(ctx context.Context, albumID string) (models.Album, error)
	ListAlbums(ctx context.Context) ([]models.Album, error)
}

type putAlbumRequest struct {
	AlbumID     *string `json:"album_id"`
	Title       *string `json:"title"`
	Description *string `json:"description"`
	Owner       *string `json:"owner"`
}

// registerAlbumRoutes wires the album endpoints to the storage boundary.
func registerAlbumRoutes(router *gin.Engine, albums AlbumStore) {
	router.PUT("/albums/:album_id", func(c *gin.Context) {
		if albums == nil {
			writeError(c, http.StatusInternalServerError, "album store not configured")
			return
		}

		pathAlbumID := c.Param("album_id")

		var req putAlbumRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			writeError(c, http.StatusBadRequest, "invalid request body")
			return
		}

		album := models.Album{AlbumID: pathAlbumID}
		if req.AlbumID != nil {
			if *req.AlbumID != pathAlbumID {
				writeError(c, http.StatusBadRequest, "album_id mismatch")
				return
			}
			album.AlbumID = *req.AlbumID
		}

		existing, err := albums.GetAlbum(c.Request.Context(), album.AlbumID)
		switch {
		case err == nil:
			album.Title = existing.Title
			album.Description = existing.Description
			album.Owner = existing.Owner
		case !errors.Is(err, store.ErrNotFound):
			writeError(c, http.StatusInternalServerError, "internal error")
			return
		}

		if req.Title != nil {
			album.Title = *req.Title
		}
		if req.Description != nil {
			album.Description = *req.Description
		}
		if req.Owner != nil {
			album.Owner = *req.Owner
		}

		saved, err := albums.PutAlbum(c.Request.Context(), album)
		if err != nil {
			writeError(c, http.StatusInternalServerError, "internal error")
			return
		}

		c.JSON(http.StatusOK, saved)
	})

	router.GET("/albums/:album_id", func(c *gin.Context) {
		if albums == nil {
			writeError(c, http.StatusInternalServerError, "album store not configured")
			return
		}

		album, err := albums.GetAlbum(c.Request.Context(), c.Param("album_id"))
		if errors.Is(err, store.ErrNotFound) {
			writeError(c, http.StatusNotFound, "not found")
			return
		}
		if err != nil {
			writeError(c, http.StatusInternalServerError, "internal error")
			return
		}

		c.JSON(http.StatusOK, album)
	})

	router.GET("/albums", func(c *gin.Context) {
		if albums == nil {
			writeError(c, http.StatusInternalServerError, "album store not configured")
			return
		}

		albumsList, err := albums.ListAlbums(c.Request.Context())
		if err != nil {
			writeError(c, http.StatusInternalServerError, "internal error")
			return
		}
		if albumsList == nil {
			albumsList = []models.Album{}
		}

		c.JSON(http.StatusOK, albumsList)
	})
}

// writeError keeps API error responses consistent across handlers.
func writeError(c *gin.Context, status int, message string) {
	c.JSON(status, models.ErrorResponse{Error: message})
}
