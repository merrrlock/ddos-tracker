package main

import (
	"encoding/json"
	"log"
	"os"
	"time"
)

type AlertState struct {
	IsActive    bool
	TriggeredAt time.Time
}

var activeAlerts = make(map[uint]AlertState)

const (
	ThresholdCPU  = 90.0
	ThresholdRAM  = 95.0
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
	DB.Find(&servers)

	log.Printf("[DEBUG] Воркер запущен. Найдено серверов в БД: %d\n", len(servers))

	promURL := os.Getenv("PROMETHEUS_URL")
	if promURL == "" {
		log.Println("Ошибка: не задан PROMETHEUS_URL")
		return
	}

	for _, server := range servers {
		cpuQuery := `dgop_cpu_usage_percent{server="alexusdot-asustufgaminga15fa506ncrfa506ncr"}`
		cpuUsage, err := QueryPrometheus(promURL, cpuQuery)
		if err != nil {
			log.Printf("[DEBUG] Пропуск сервера %s. Ошибка Prometheus: %v\n", server.IPAddress, err)
			continue
		}

		ramQuery := `dgop_memory_used_percent{server="alexusdot-asustufgaminga15fa506ncrfa506ncr"}`
		ramUsage, err := QueryPrometheus(promURL, ramQuery)
		if err != nil {
			log.Printf("[DEBUG] Ошибка получения RAM для %s: %v\n", server.IPAddress, err)
			continue
		}

		log.Printf("[DEBUG] Успех! Сервер %s -> CPU: %.2f%%, RAM: %.2f%%\n", server.IPAddress, cpuUsage, ramUsage)

		currentMetric := SystemMetric{
			CPU: cpuUsage,
			RAM: ramUsage,
		}

		state := activeAlerts[server.ID]

		if currentMetric.CPU > ThresholdCPU || currentMetric.RAM > ThresholdRAM {
			if !state.IsActive {
				log.Printf("🔥 [CRITICAL] СРАБОТАЛ АЛЕРТ! CPU: %.2f%% > Порога %.2f%%", currentMetric.CPU, ThresholdCPU)
				activeAlerts[server.ID] = AlertState{IsActive: true, TriggeredAt: time.Now()}
				triggerAlert(server, currentMetric)
			}
		} else {
			if state.IsActive {
				log.Printf("✅ [RESOLVED] Нагрузка спала. CPU: %.2f%%", currentMetric.CPU)
				activeAlerts[server.ID] = AlertState{IsActive: false}
				resolveAlert(server)
			}
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
