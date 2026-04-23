package main

import (
	"log"
	"time"
)

// анти-флуд
type AlertState struct {
	IsActive  bool
	LastAlert time.Time
}

// Кэш состояний: ключ - ServerID, значение - состояние алерта
var activeAlerts = make(map[uint]AlertState)

// Пороги срабатывания (позже вынесем в БД)
const (
	ThresholdCPU  = 90.0
	ThresholdRAM  = 85.0
	ThresholdDisk = 95.0
)

// Запуск фонового воркера (будет работать параллельно с API)
func StartAlertWorker(interval time.Duration) {
	ticker := time.NewTicker(interval)

	// Бесконечный цикл в отдельной горутине
	go func() {
		for {
			select {
			case <-ticker.C:
				checkMetricsForAlerts()
			}
		}
	}()

	log.Println("Alert Worker успешно запущен!")
}

// Основная логика проверки
func checkMetricsForAlerts() {
	var servers []Server
	// Получаем список всех активных серверов
	if err := DB.Where("status = ?", "Online").Find(&servers).Error; err != nil {
		log.Println("Ошибка получения серверов для алертов:", err)
		return
	}

	for _, server := range servers {
		var latestMetric SystemMetric

		result := DB.Where("server_id = ?", server.ID).Order("id desc").First(&latestMetric)

		if result.Error != nil {
			continue
		}

		isBreached := latestMetric.CPU > ThresholdCPU ||
			latestMetric.RAM > ThresholdRAM ||
			latestMetric.Disk > ThresholdDisk

		state := activeAlerts[server.ID]

		if isBreached && !state.IsActive {
			triggerAlert(server, latestMetric)

			activeAlerts[server.ID] = AlertState{IsActive: true, LastAlert: time.Now()}

		} else if !isBreached && state.IsActive {
			resolveAlert(server)

			activeAlerts[server.ID] = AlertState{IsActive: false}
		}
	}
}

func triggerAlert(server Server, metric SystemMetric) {
	log.Printf("[CRITICAL] Сервер %s (%s) превысил пороги! CPU: %.1f%%, RAM: %.1f%%\n",
		server.Name, server.IPAddress, metric.CPU, metric.RAM)

}

func resolveAlert(server Server) {
	log.Printf("[RESOLVED] Сервер %s (%s) вернулся в штатный режим.\n", server.Name, server.IPAddress)

}
