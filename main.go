package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"log"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/sirupsen/logrus"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// @title Music info
// @version 1.0
// @description This is a music information API.
// @termsOfService http://swagger.io/terms/

// @contact.name API Support
// @contact.url http://www.swagger.io/support
// @contact.email support@swagger.io

// @license.name Apache 2.0
// @license.url http://www.apache.org/licenses/LICENSE-2.0.html

// @BasePath /api/v1

// Структура Song (Песня)
type Song struct {
	ID          int    `json:"id" gorm:"primaryKey"`
	Group       string `json:"group" binding:"required"`
	SongName    string `json:"song" binding:"required"`
	ReleaseDate string `json:"releaseDate"`
	Text        string `json:"text"`
	Link        string `json:"link"`
}

var db *gorm.DB

func GetDB() *gorm.DB {
	if db == nil {
		err := godotenv.Load()
		if err != nil {
			log.Fatal("Error loading .env file")
		}
		dbConn, err := gorm.Open(postgres.Open(os.Getenv("DATABASE_URL")), &gorm.Config{})
		if err != nil {
			log.Fatalf("Failed to connect to database: %v", err)
		}
		db = dbConn
	}
	return db
}

func main() {
	gin.SetMode(gin.ReleaseMode)

	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	dsn := os.Getenv("DATABASE_URL")                         // Получение DSN из переменной окружения
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{}) // Использование postgres.Open()
	sqlDB, err := db.DB()                                    // Получение базового соединения *sql.DB
	if err != nil {
		log.Fatal(err)
	}
	defer sqlDB.Close()

	router := gin.Default()

	router.GET("/songs", GetSongs)
	router.POST("/songs", AddSong)
	router.PUT("/songs/:id", UpdateSong)
	router.DELETE("/songs/:id", DeleteSong)
	router.GET("/songs/:id/text", GetSongText)

}

// @Summary Get songs
// @Description Get a list of songs.
// @ID get-songs
// @Accept json
// @Produce json
// @Param page query int false "Page number"
// @Param limit query int false "Limit number"
// @Success 200 {array} Song
// @Failure 500 {object} Error
// func GetSongs(c *gin.Context)

func Migrate(db *gorm.DB) error {
	err := db.AutoMigrate(&Song{})
	if err != nil {
		fmt.Errorf("failed to migrate database: %v", err)
		return err
	}
	return nil
}

// @Summary Get songs
// @Description Get a list of songs.
// @ID get-songs
// @Accept  json
// @Produce  json
// @Param page query int false "Page number"
// @Param limit query int false "Limit number"
// @Param group query string false "Group filter"
// @Param song query string false "Song filter"
// @Param releaseDate query string false "Release date filter"
// @Param text query string false "Text filter"
// @Param link query string false "Link filter"
// @Success 200 {array} Song
// @Failure 500 {object} Error

func GetSongs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	offset := (page - 1) * limit

	var song Song
	if err := c.ShouldBindQuery(&song); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var songs []Song
	db := GetDB()
	query := db.Model(&Song{})

	// Создаем map для условий фильтрации
	where := make(map[string]interface{})
	if song.Group != "" {
		query = query.Where("group = ?", song.Group)
	}
	if song.SongName != "" {
		query = query.Where("song_name = ?", song.SongName)
	}
	if song.ReleaseDate != "" {
		query = query.Where("release_date = ?", song.ReleaseDate)
	}
	if song.Text != "" {
		query = query.Where("text LIKE ?", "%"+song.Text+"%")
	}
	if song.Link != "" {
		query = query.Where("link = ?", song.Link)
	}

	result := db.Model(&Song{}).Where(where).Offset(offset).Limit(limit).Find(&songs)

	if result.Error != nil {
		logrus.WithError(result.Error).Error("Failed to fetch songs from database")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch songs"})
		return
	}
	if len(songs) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "No songs found"})
		return
	}
	c.JSON(http.StatusOK, songs)
}

// @Summary Add song
// @Description Add a new song.
// @ID add-song
// @Accept  json
// @Produce  json
// @Param song body Song true "Song object"
// @Success 201 {object} Song
// @Failure 400 {object} Error
// @Failure 500 {object} Error

func AddSong(c *gin.Context) {
	var newSong Song
	if err := c.ShouldBindBodyWithJSON(&newSong); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := http.Get(fmt.Sprintf("http://localhost:8080/info?group=%s&song=%s", url.QueryEscape(newSong.Group), url.QueryEscape(newSong.SongName)))
	if err != nil {
		logrus.WithError(err).Error("Failed to fetch song info from external API")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch songs"})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logrus.WithField("status_code", resp.StatusCode).Error("External API returned non-OK status code")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add song"})
		return
	}
	var songDetail SongDetail
	if err := json.NewDecoder(resp.Body).Decode(&songDetail); err != nil {
		logrus.WithError(err).Error("Failed to decode song info from external API")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch song info"})
		return
	}

	verses := strings.Split(songDetail.Text, "\n\n")
	newSong.Text = strings.Join(verses, "\n\n")

	newSong.ReleaseDate = songDetail.ReleaseDate
	newSong.Text = songDetail.Text
	newSong.Link = songDetail.Link

	db := GetDB()
	if err := db.Create(&newSong).Error; err != nil {
		logrus.WithError(err).Error("Failed to create song in database")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add song"})
		return
	}

	c.JSON(http.StatusCreated, newSong)

}

type SongDetail struct {
	ReleaseDate string `json:"releaseDate"`
	Text        string `json:"text"`
	Link        string `json:"link"`
}

// @Summary Update song
// @Description Update a song.
// @ID update-song
// @Accept  json
// @Produce  json
// @Param id path int true "Song ID"
// @Param song body Song true "Song object"
// @Success 200 {object} Song
// @Failure 400 {object} Error
// @Failure 404 {object} Error
// @Failure 500 {object} Error

func UpdateSong(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid song ID"})
		return
	}

	var song Song
	if err := c.ShouldBindJSON(&song); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	db := GetDB()
	result := db.Model(&Song{}).Where("id = ?", id).Updates(&song)

	if result.Error != nil {
		logrus.WithError(result.Error).Error("Failed to update song")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update song"})
		return
	}

	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Song not found"})
		return
	}

	c.JSON(http.StatusOK, song)
}

// @Summary Delete song
// @Description Delete a song.
// @ID delete-song
// @Accept  json
// @Produce  json
// @Param id path int true "Song ID"
// @Success 200 {object} Message
// @Failure 404 {object} Error
// @Failure 500 {object} Error

func DeleteSong(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid song ID"})
		return
	}

	db := GetDB()
	result := db.Where("id = ?", id).Delete(&Song{})

	if result.Error != nil {
		logrus.WithError(result.Error).Error("Failed to delete song")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete song"})
		return
	}

	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Song not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Song deleted"})
}

func GetSongText(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid song ID"})
		return
	}

	var song Song
	db := GetDB()
	result := db.First(&song, id)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Song not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch song"})
			logrus.WithError(result.Error).Error("Error fetching song text")
		}
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	offset := (page - 1) * limit
	end := min(offset+limit, len(song.Text))
	text := song.Text[offset:end]

	c.JSON(http.StatusOK, gin.H{"text": text})
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
