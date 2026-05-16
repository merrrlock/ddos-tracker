package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/coreos/go-systemd/v22/sdjournal"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// AllResponse отражает реальный JSON, который возвращает dgop
type AllResponse struct {
	CPU struct {
		Usage       float64     `json:"usage"`
		Temperature float64     `json:"temperature"`
		CoreUsage   []float64   `json:"coreUsage"`   // массив процентов загрузки ядер
	} `json:"cpu"`
	Memory struct {
		UsedPercent float64 `json:"usedPercent"`
		Used        uint64  `json:"used"`
	} `json:"memory"`
	Network []struct {
		Name string `json:"name"`
		Rx   uint64 `json:"rx"`
		Tx   uint64 `json:"tx"`
	} `json:"network"`
	Disk []struct {
		Name  string `json:"name"`
		Read  uint64 `json:"read"`
		Write uint64 `json:"write"`
	} `json:"disk"`
}

func main() {
	hostname, _ := os.Hostname()

	// CPU / память
	cpuUsage := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "dgop_cpu_usage_percent", 
			Help: "Current CPU usage from dgop"},
		[]string{"server"},
	)
	cpuTemperature := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "dgop_cpu_temperature_celsius", 
			Help: "Current CPU temperature"},
		[]string{"server"},
	)
	memUsedPercent := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "dgop_memory_used_percent", 
			Help: "Current used memory in percent"},
		[]string{"server"},
	)
	memUsingNow := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "dgop_memory_using_now", 
			Help: "Memory that is used now"},
		[]string{"server"},
	)

	// Сеть (Вычисляется по разнице rx/tx)
	ppsIn := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "dgop_pps_in",
			Help: "Packets per second received"},
		[]string{"server", "device"},
	)
	ppsOut := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "dgop_pps_out", 
			Help: "Packets per second sent"},
		[]string{"server", "device"},
	)
	bpsIn := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "dgop_bps_in", 
			Help: "Bits per second received"},
		[]string{"server", "device"},
	)
	bpsOut := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "dgop_bps_out", 
			Help: "Bits per second sent"},
		[]string{"server", "device"},
	)

	// Диск
	diskRead := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "dgop_disk_read_bytes", 
			Help: "Disk read bytes"},
		[]string{"server", "device"},
	)
	diskWrite := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "dgop_disk_write_bytes", 
			Help: "Disk write bytes"},
		[]string{"server", "device"},
	)

	// Ядра
	coreDiff := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "dgop_core_usage_percent", 
			Help: "Per-core CPU usage"},
		[]string{"server", "cpu"},
	)

	// Вход/выход
	loginEvents := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "dgop_login_events_total", 
			Help: "Total login events"},
		[]string{"server", "user"},
	)
	logoutEvents := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "dgop_logout_events_total", 
			Help: "Total logout events"},
		[]string{"server", "user"},
	)
	activeUsers := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "dgop_logged_in_users", 
			Help: "Current logged in users"},
		[]string{"server"},
	)

	//Регистрация метрик в prometheus
	prometheus.MustRegister(
		cpuUsage, cpuTemperature,
		memUsedPercent, memUsingNow,
		ppsIn, ppsOut, bpsIn, bpsOut,
		diskRead, diskWrite,
		coreDiff,
		loginEvents, logoutEvents, activeUsers,
	)

	// горутина сбора метрик
	go func() {
		// Предыдущие значения для расчёта pps/bps (накапливаем rx/tx в байтах)
		prevNet := map[string]struct{ Rx, Tx uint64 }{}

		url := "http://localhost:63484/gops/meta?modules=cpu,memory,network,disk"

		for {
			resp, err := http.Get(url)
			if err != nil {
				log.Printf("dgop error: %v", err)
				time.Sleep(15 * time.Second)
				continue
			}

			var data AllResponse
			if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
				log.Printf("json error: %v", err)
				resp.Body.Close()
				time.Sleep(15 * time.Second)
				continue
			}
			resp.Body.Close()

			// Базовые метрики
			cpuUsage.WithLabelValues(hostname).Set(data.CPU.Usage)
			cpuTemperature.WithLabelValues(hostname).Set(data.CPU.Temperature)
			memUsedPercent.WithLabelValues(hostname).Set(data.Memory.UsedPercent)
			memUsingNow.WithLabelValues(hostname).Set(float64(data.Memory.Used))

			// Сеть: считаю pps/bps как разницу за 15 секунд
			for _, iface := range data.Network {
				prev, ok := prevNet[iface.Name]
				if ok {
					// Разница в байтах
					deltaRx := float64(iface.Rx - prev.Rx)
					deltaTx := float64(iface.Tx - prev.Tx)
					// За секунду
					rxBps := deltaRx / 15.0
					txBps := deltaTx / 15.0
					// Переводим в биты (*8)
					bpsIn.WithLabelValues(hostname, iface.Name).Set(rxBps * 8)
					bpsOut.WithLabelValues(hostname, iface.Name).Set(txBps * 8)
					// Пакеты не отдаются, ставим 0 (или вычислите позже через отдельный запрос)
					ppsIn.WithLabelValues(hostname, iface.Name).Set(0)
					ppsOut.WithLabelValues(hostname, iface.Name).Set(0)
				}
				// Сохраняем текущие значения
				prevNet[iface.Name] = struct{ Rx, Tx uint64 }{iface.Rx, iface.Tx}
			}

			// Диск (read/write напрямую)
			for _, dev := range data.Disk {
				diskRead.WithLabelValues(hostname, dev.Name).Set(float64(dev.Read))
				diskWrite.WithLabelValues(hostname, dev.Name).Set(float64(dev.Write))
			}

			// Загрузка ядер
			for i, usage := range data.CPU.CoreUsage {
				coreDiff.WithLabelValues(hostname, fmt.Sprintf("%d", i)).Set(usage)
			}

			log.Printf("CPU: %.2f%% | Temp: %.1f°C | Mem: %.2f%% | Net: %d devs | Cores: %d",
				data.CPU.Usage, data.CPU.Temperature,
				data.Memory.UsedPercent,
				len(data.Network), len(data.CPU.CoreUsage),
			)

			time.Sleep(15 * time.Second)
		}
	}()

	go startJournalWatcher(hostname, loginEvents, logoutEvents, activeUsers)

	log.Println("Агент запущен на :8080")
	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(":8080", nil))
}

// прочее: startJournalWatcher, extractUser, getActiveUsers
func startJournalWatcher(
	hostname string,
	loginEvents *prometheus.CounterVec,
	logoutEvents *prometheus.CounterVec,
	activeUsers *prometheus.GaugeVec,
) {
	j, err := sdjournal.NewJournal()
	if err != nil {
		log.Printf("Ошибка журнала: %v", err)
		return
	}

	j.AddMatch("_SYSTEMD_UNIT=sshd.service")
	j.SeekTail()
	j.Next()

	for {
		n, err := j.Next()
		if err != nil {
			log.Printf("Ошибка чтения журнала %v", err)
			continue
		}

		if n == 0 {
			j.Wait(1 * time.Second)
			continue
		}

		entry, err := j.GetEntry()
		if err != nil {
			continue
		}

		msg := entry.Fields["MESSAGE"]

		if strings.Contains(msg, "Accepted") {
			user := extractUser(msg)
			loginEvents.WithLabelValues(hostname, user).Inc()
			log.Printf("[LOGIN] %s", user)
		}

		if strings.Contains(msg, "session closed") {
			user := extractUser(msg)
			logoutEvents.WithLabelValues(hostname, user).Inc()
			log.Printf("[LOGOUT] %s", user)
		}

		count := getActiveUsers()
		activeUsers.WithLabelValues(hostname).Set(float64(count))
	}
}

func extractUser(msg string) string {
	parts := strings.Split(msg, " ")
	for i := range parts {
		if parts[i] == "for" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return "unknown"
}

func getActiveUsers() int {
	out, err := exec.Command("who").Output()
	if err != nil {
		return 0
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return 0
	}
	return len(lines)
}