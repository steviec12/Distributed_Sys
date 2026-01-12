package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var db *gorm.DB // Global database connection

func main() {
	initDB() // connect to database

	router := gin.Default()
	router.GET("/albums", getAlbums)
	router.POST("/albums", postAlbums)
	router.GET("/albums/:id", getAlbumByID)
	router.PUT("/albums/:id", updateAlbum)
	router.DELETE("/albums/:id", deleteAlbum)
	router.Run("localhost:8080")

}

func getAlbums(c *gin.Context) {
	var albums []album
	db.Find(&albums) //pass in address for album data
	c.IndentedJSON(http.StatusOK, albums)
}

func postAlbums(c *gin.Context) {
	var newAlbum album

	// check for error for parsing new album
	if err := c.BindJSON(&newAlbum); err != nil {
		return
	}
	db.Create(&newAlbum) // into database
	c.IndentedJSON(http.StatusCreated, newAlbum)
}

func getAlbumByID(c *gin.Context) {
	id := c.Param("id")
	var album album
	result := db.First(&album, "id = ?", id)
	if result.Error != nil {
		c.IndentedJSON(http.StatusNotFound, gin.H{"message": "album not found"})
		return
	}
	c.IndentedJSON(http.StatusOK, album)

}

func updateAlbum(c *gin.Context) {
	id := c.Param("id")
	var album album
	result := db.First(&album, "id = ?", id)
	if result.Error != nil {
		c.IndentedJSON(http.StatusNotFound, gin.H{"message": "album not found"})
		return
	}

	if err := c.BindJSON(&album); err != nil {
		return
	}

	album.ID = id

	db.Save(&album)
	c.IndentedJSON(http.StatusOK, album)

}

func deleteAlbum(c *gin.Context) {
	id := c.Param("id")

	var album album
	result := db.First(&album, "id = ?", id)
	if result.Error != nil {
		c.IndentedJSON(http.StatusNotFound, gin.H{"message": "album not found"})
		return
	}
	db.Delete(&album, "id = ?", id)
	c.Status(http.StatusNoContent)
}

type album struct {
	ID     string  `json:"id" gorm:"primaryKey"`
	Title  string  `json:"title"`
	Artist string  `json:"artist"`
	Price  float64 `json:"price"`
}

func initDB() {
	var err error // error indicator

	// first part create the album database second part is defaut option for configuration
	db, err = gorm.Open(sqlite.Open("album.db"), &gorm.Config{})

	// if there is an error crash the program with error message
	if err != nil {
		panic("failed to connect to database")
	}

	//check/update data schema
	db.AutoMigrate(&album{})
}
