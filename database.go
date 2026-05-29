package main

import (
	"log"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var DB *gorm.DB

type User struct {
	ID           uint   `gorm:"primaryKey"`
	Email        string `gorm:"unique;not null"`
	PasswordHash string `gorm:"not null"`
	Role         string `gorm:"default:'user'"`
	TelegramID   int64
}

type Server struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	Name      string         `json:"name"`
	IPAddress string         `json:"ip_address"`
	APIKey    string         `json:"api_key"`
	Metrics   []SystemMetric `json:"metrics" gorm:"foreignKey:ServerID"`
}

type SystemMetric struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	ServerID  uint      `json:"server_id"`
	CPU       float64   `json:"cpu"`
	RAM       float64   `json:"ram"`
	Disk      float64   `json:"disk"`
	NetIn     int64     `json:"net_in"`
	NetOut    int64     `json:"net_out"`
	CreatedAt time.Time `gorm:"autoCreateTime;index"`
}

type AlertLog struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	ServerID  uint      `json:"server_id"`
	ServerIP  string    `json:"server_ip"`
	Type      string    `json:"type"`
	CPU       float64   `json:"cpu"`
	RAM       float64   `json:"ram"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

func InitDB(dsn string) {
	var err error
	DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatal("Ошибка подключения к базе данных:", err)
	}

	log.Println("Успешное подключение к PostgreSQL!")

	err = DB.AutoMigrate(&Server{}, &SystemMetric{}, &AlertLog{})
	if err != nil {
		log.Fatal("Ошибка при выполнении миграции:", err)
	}

	log.Println("Таблицы успешно синхронизированы.")

	DB.Exec("CREATE EXTENSION IF NOT EXISTS timescaledb;")
	err = DB.Exec("SELECT create_hypertable('system_metrics', by_range('created_at'), if_not_exists => TRUE);").Error

	if err != nil {
		log.Println("Заметка TimescaleDB:", err)
	} else {
		log.Println("TimescaleDB: Таблица system_metrics готова к сверхбыстрой записи!")
	}
}
