package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type IncomingMetrics struct {
	APIKey string  `json:"api_key" binding:"required"`
	CPU    float64 `json:"cpu" binding:"required"`
	RAM    float64 `json:"ram" binding:"required"`
	Disk   float64 `json:"disk" binding:"required"`
	NetIn  uint64  `json:"net_in"`
	NetOut uint64  `json:"net_out"`
}

type CreateServerRequest struct {
	Name      string `json:"name" binding:"required"`
	IPAddress string `json:"ip_address" binding:"required"`
}

func main() {
	dsn := "host=localhost user=postgres password=root dbname=monitoring_db port=5432 sslmode=disable"

	InitDB(dsn)

	router := gin.Default()

	api := router.Group("/api")
	{
		api.GET("/health", healthCheck)
		api.POST("/servers", createServer)
		api.POST("/metrics", receiveMetrics)
	}

	router.Run(":8080")
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

	newServer := Server{
		Name:      req.Name,
		IPAddress: req.IPAddress,
		APIKey:    "generated-key-12345",
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

func receiveMetrics(c *gin.Context) {
	var req IncomingMetrics

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный формат данных: " + err.Error()})
		return
	}

	var server Server
	result := DB.Where("api_key = ?", req.APIKey).First(&server)

	if result.Error != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Неверный API ключ"})
		return
	}

	metric := SystemMetric{
		ServerID: server.ID,
		CPU:      req.CPU,
		RAM:      req.RAM,
		Disk:     req.Disk,
		NetIn:    req.NetIn,
		NetOut:   req.NetOut,
	}

	if err := DB.Create(&metric).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка сохранения метрик"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
