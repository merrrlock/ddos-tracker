package main

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

type CreateServerRequest struct {
	Name      string `json:"name" binding:"required"`
	IPAddress string `json:"ip_address" binding:"required"`
}

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("Файл ..env не найден, используем системные переменные окружения")
	}

	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		log.Fatal("Критическая ошибка: не задана переменная DB_DSN")
	}

	InitDB(dsn)
	InitRedis()

	StartAlertWorker(10 * time.Second)
	StartEmailWorker()

	router := gin.Default()

	router.GET("/ws", handleConnections)

	router.Static("/static", "./public")

	router.StaticFile("/", "public/index.html")

	router.GET("/ws/alerts", WsAlertsHandler)

	api := router.Group("/api")
	{
		api.GET("/health", healthCheck)
		api.POST("/servers", createServer)
		api.GET("/servers", getServers)
		api.POST("/metrics", receiveMetrics)
	}

	err := router.Run(":8080")
	if err != nil {
		return
	}
}

func healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "API online", "database": "connected"})
}

func createServer(c *gin.Context) {
	var req CreateServerRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный формат данных: " + err.Error()})
		return
	}

	apiKey, err := generateAPIKey(16)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка генерации API-ключа"})
		return
	}

	newServer := Server{
		Name:      req.Name,
		IPAddress: req.IPAddress,
		APIKey:    apiKey,
	}

	result := DB.Create(&newServer)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Не удалось создать сервер"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":   "Сервер успешно добавлен",
		"server_id": newServer.ID,
		"api_key":   newServer.APIKey,
	})
}

func getServers(c *gin.Context) {
	var servers []Server

	if err := DB.Find(&servers).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Не удалось получить список серверов: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, servers)
}

// Вспомогательная функция для генерации случайных токенов
func generateAPIKey(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
