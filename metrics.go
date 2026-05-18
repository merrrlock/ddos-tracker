package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type MetricPayload struct {
	APIKey string  `json:"api_key" binding:"required"`
	CPU    float64 `json:"cpu"`
	RAM    float64 `json:"ram"`
	Disk   float64 `json:"disk"`
	NetIn  int64   `json:"net_in"`
	NetOut int64   `json:"net_out"`
}

func receiveMetrics(c *gin.Context) {
	var payload MetricPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный формат метрик"})
		return
	}

	// 1. Ищем сервер по API-ключу
	var server Server
	if err := DB.Where("api_key = ?", payload.APIKey).First(&server).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Неверный API-ключ"})
		return
	}

	// 2. Создаем и сохраняем метрику (убедись, что структура SystemMetric у тебя выглядит похоже)
	metric := SystemMetric{
		ServerID: server.ID,
		CPU:      payload.CPU,
		RAM:      payload.RAM,
		// Disk, NetIn, NetOut...
	}
	DB.Create(&metric)

	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Метрики приняты"})
}
