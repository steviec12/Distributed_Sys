package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"path"

	"album-store-v1/internal/models"
	"album-store-v1/internal/store"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// PhotoStore captures the photo and job operations the HTTP layer depends on.
type PhotoStore interface {
	GetPhoto(ctx context.Context, albumID, photoID string) (models.Photo, error)
	GetPhotoForDelete(ctx context.Context, albumID, photoID string) (models.Photo, error)
	CreatePhotoProcessing(ctx context.Context, albumID, photoID, s3Key, tempPath, contentType string) (models.Photo, error)
	MarkPhotoFailed(ctx context.Context, albumID, photoID string) error
	MarkPhotoDeleted(ctx context.Context, albumID, photoID string) (models.Photo, error)
}

// ObjectStorage abstracts the S3 operations used by upload, status, and delete.
type ObjectStorage interface {
	DeleteObject(ctx context.Context, key string) error
	PublicURL(key string) string
}

// TempFileStorage stores accepted uploads locally until the worker pushes them to S3.
type TempFileStorage interface {
	Save(body io.Reader, filename string) (string, error)
	Delete(path string) error
}

// registerPhotoRoutes wires the upload, status, and delete endpoints.
func registerPhotoRoutes(router *gin.Engine, photos PhotoStore, objects ObjectStorage, tempFiles TempFileStorage, jobs JobSubmitter, newID func() string) {
	idGenerator := newID
	if idGenerator == nil {
		idGenerator = uuid.NewString
	}

	router.POST("/albums/:album_id/photos", func(c *gin.Context) {
		if photos == nil {
			writeError(c, http.StatusInternalServerError, "photo store not configured")
			return
		}
		if objects == nil {
			writeError(c, http.StatusInternalServerError, "object storage not configured")
			return
		}
		if tempFiles == nil {
			writeError(c, http.StatusInternalServerError, "temp file storage not configured")
			return
		}
		if jobs == nil {
			writeError(c, http.StatusInternalServerError, "photo pipeline not configured")
			return
		}

		reader, err := c.Request.MultipartReader()
		if err != nil {
			writeError(c, http.StatusBadRequest, "missing multipart body")
			return
		}

		var part *multipart.Part
		for {
			p, err := reader.NextPart()
			if err != nil {
				writeError(c, http.StatusBadRequest, "missing photo field")
				return
			}
			if p.FormName() == "photo" {
				part = p
				break
			}
			p.Close()
		}
		defer part.Close()

		albumID := c.Param("album_id")
		photoID := idGenerator()
		filename := part.FileName()
		contentType := part.Header.Get("Content-Type")
		s3Key := buildPhotoObjectKey(albumID, photoID, filename)

		tempPath, err := tempFiles.Save(part, filename)
		if err != nil {
			log.Printf("photo upload temp file failure album_id=%s photo_id=%s s3_key=%s err=%v", albumID, photoID, s3Key, err)
			writeError(c, http.StatusInternalServerError, "internal error")
			return
		}

		log.Printf(
			"photo upload start album_id=%s photo_id=%s filename=%q content_type=%q s3_key=%s",
			albumID, photoID, filename, contentType, s3Key,
		)

		photo, err := photos.CreatePhotoProcessing(c.Request.Context(), albumID, photoID, s3Key, tempPath, contentType)
		if err != nil {
			_ = tempFiles.Delete(tempPath)
			if errors.Is(err, store.ErrNotFound) {
				writeError(c, http.StatusNotFound, "not found")
				return
			}
			writeError(c, http.StatusInternalServerError, "internal error")
			return
		}

		if err := jobs.Submit(photo.PhotoID); err != nil {
			_ = tempFiles.Delete(tempPath)
			_ = photos.MarkPhotoFailed(c.Request.Context(), albumID, photoID)
			writeError(c, http.StatusInternalServerError, "internal error")
			return
		}

		log.Printf("photo upload accepted album_id=%s photo_id=%s seq=%d", albumID, photoID, photo.Seq)

		c.JSON(http.StatusAccepted, models.PhotoAcceptedResponse{
			PhotoID: photo.PhotoID,
			Seq:     photo.Seq,
			Status:  photo.Status,
		})
	})

	router.GET("/albums/:album_id/photos/:photo_id", func(c *gin.Context) {
		if photos == nil {
			writeError(c, http.StatusInternalServerError, "photo store not configured")
			return
		}

		photo, err := photos.GetPhoto(c.Request.Context(), c.Param("album_id"), c.Param("photo_id"))
		if errors.Is(err, store.ErrNotFound) {
			writeError(c, http.StatusNotFound, "not found")
			return
		}
		if err != nil {
			writeError(c, http.StatusInternalServerError, "internal error")
			return
		}

		c.JSON(http.StatusOK, photo.StatusResponse())
	})

	router.DELETE("/albums/:album_id/photos/:photo_id", func(c *gin.Context) {
		if photos == nil {
			writeError(c, http.StatusInternalServerError, "photo store not configured")
			return
		}
		if objects == nil {
			writeError(c, http.StatusInternalServerError, "object storage not configured")
			return
		}

		albumID := c.Param("album_id")
		photoID := c.Param("photo_id")

		photo, err := photos.GetPhotoForDelete(c.Request.Context(), albumID, photoID)
		if errors.Is(err, store.ErrNotFound) {
			c.Status(http.StatusNoContent)
			return
		}
		if err != nil {
			writeError(c, http.StatusInternalServerError, "internal error")
			return
		}

		if !photo.Deleted {
			deletedPhoto, err := photos.MarkPhotoDeleted(c.Request.Context(), albumID, photoID)
			if err != nil {
				if !errors.Is(err, store.ErrNotFound) {
					writeError(c, http.StatusInternalServerError, "internal error")
					return
				}
			} else {
				photo = deletedPhoto
			}
		}

		if tempFiles != nil && photo.TempPath != "" {
			if err := tempFiles.Delete(photo.TempPath); err != nil {
				log.Printf("photo delete temp cleanup failure album_id=%s photo_id=%s temp_path=%s err=%v", albumID, photoID, photo.TempPath, err)
			}
		}

		if err := objects.DeleteObject(c.Request.Context(), photo.S3Key); err != nil {
			writeError(c, http.StatusInternalServerError, "internal error")
			return
		}

		c.Status(http.StatusNoContent)
	})
}

// buildPhotoObjectKey distributes objects across S3 partitions using a hash prefix
// derived from the photo ID so uploads to the same album don't hotspot one partition.
func buildPhotoObjectKey(albumID, photoID, filename string) string {
	return fmt.Sprintf("%s/albums/%s/photos/%s%s", photoID[:4], albumID, photoID, path.Ext(filename))
}
