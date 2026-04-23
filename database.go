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
	ID        uint   `gorm:"primaryKey"`
	Name      string `gorm:"not null"`
	IPAddress string `gorm:"not null"`
	APIKey    string `gorm:"unique;not null"`
	Status    string `gorm:"default:'Online'"`
	OwnerID   uint
}

type SystemMetric struct {
	ID        uint    `gorm:"primaryKey"`
	ServerID  uint    `gorm:"not null;index"`
	CPU       float64 `gorm:"not null"`
	RAM       float64 `gorm:"not null"`
	Disk      float64 `gorm:"not null"`
	NetIn     uint64
	NetOut    uint64
	CreatedAt time.Time `gorm:"autoCreateTime;index"`
}

func InitDB(dsn string) {
	var err error
	DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatal("Ошибка подключения к базе данных:", err)
	}

	log.Println("Успешное подключение к PostgreSQL!")

	err = DB.AutoMigrate(&User{}, &Server{}, &SystemMetric{})
	if err != nil {
		log.Fatal("Ошибка при выполнении миграции:", err)
	}

	log.Println("Таблицы успешно синхронизированы.")
}
