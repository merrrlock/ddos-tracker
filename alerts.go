package main

import (
	"encoding/json"
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

const (
	ThresholdCPU  = 90.0
	ThresholdRAM  = 85.0
	ThresholdDisk = 95.0
)

func StartAlertWorker(interval time.Duration) {
	ticker := time.NewTicker(interval)

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

func checkMetricsForAlerts() {
	var servers []Server
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

type AlertMessage struct {
	Type       string  `json:"type"`
	ServerName string  `json:"server_name"`
	IPAddress  string  `json:"ip_address"`
	CPU        float64 `json:"cpu"`
	RAM        float64 `json:"ram"`
	Message    string  `json:"message"`
}

func triggerAlert(server Server, metric SystemMetric) {
	log.Printf("[CRITICAL] Сервер %s превысил пороги!\n", server.Name)

	msg := AlertMessage{
		Type:       "CRITICAL",
		ServerName: server.Name,
		IPAddress:  server.IPAddress,
		CPU:        metric.CPU,
		RAM:        metric.RAM,
		Message:    "Превышение допустимой нагрузки на ресурсы",
	}

	jsonData, err := json.Marshal(msg)
	if err == nil {
		RDB.Publish(Ctx, "alerts_channel", jsonData)
	}
}

func resolveAlert(server Server) {
	log.Printf("[RESOLVED] Сервер %s вернулся в штатный режим.\n", server.Name)

	msg := AlertMessage{
		Type:       "RESOLVED",
		ServerName: server.Name,
		IPAddress:  server.IPAddress,
		Message:    "Нагрузка вернулась в норму",
	}

	jsonData, err := json.Marshal(msg)
	if err == nil {
		RDB.Publish(Ctx, "alerts_channel", jsonData)
	}
}
