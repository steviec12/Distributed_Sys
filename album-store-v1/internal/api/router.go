package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// JobSubmitter enqueues accepted photo work for asynchronous completion.
type JobSubmitter interface {
	Submit(photoID string) error
}

// Dependencies collects the storage collaborators required by the HTTP layer.
type Dependencies struct {
	Albums    AlbumStore
	Photos    PhotoStore
	Objects   ObjectStorage
	TempFiles TempFileStorage
	Pipeline  JobSubmitter
	NewID     func() string
}

// NewRouter builds the full HTTP surface for the assignment contract.
func NewRouter(deps Dependencies) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	router := gin.New()
	router.Use(gin.Recovery())

	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	registerAlbumRoutes(router, deps.Albums)
	registerPhotoRoutes(router, deps.Photos, deps.Objects, deps.TempFiles, deps.Pipeline, deps.NewID)

	return router
}
