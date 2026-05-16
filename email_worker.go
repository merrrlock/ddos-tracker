package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/smtp"
	"os"
)

func StartEmailWorker() {
	pubsub := RDB.Subscribe(Ctx, "alerts_channel")

	ch := pubsub.Channel()

	log.Println("Email Worker запущен и слушает очередь алертов...")

	go func() {
		defer pubsub.Close()
		for msg := range ch {
			processAlertAndSendEmail(msg.Payload)
		}
	}()
}

func processAlertAndSendEmail(payload string) {
	var alert AlertMessage

	if err := json.Unmarshal([]byte(payload), &alert); err != nil {
		log.Println("Ошибка парсинга JSON из Redis:", err)
		return
	}

	smtpHost := os.Getenv("SMTP_HOST")
	smtpPort := os.Getenv("SMTP_PORT")
	smtpEmail := os.Getenv("SMTP_EMAIL")
	smtpPassword := os.Getenv("SMTP_PASSWORD")

	adminEmails := []string{"admin1@example.com"}

	subject := fmt.Sprintf("Subject: [%s] Инцидент на сервере %s\r\n", alert.Type, alert.ServerName)
	mime := "MIME-version: 1.0;\nContent-Type: text/plain; charset=\"UTF-8\";\n\n"

	body := fmt.Sprintf("Система мониторинга зафиксировала изменение состояния.\n\n"+
		"Сервер: %s (%s)\n"+
		"Статус: %s\n"+
		"CPU: %.1f%%\n"+
		"RAM: %.1f%%\n"+
		"Детали: %s",
		alert.ServerName, alert.IPAddress, alert.Type, alert.CPU, alert.RAM, alert.Message)

	fullMessage := []byte(subject + mime + body)

	auth := smtp.PlainAuth("", smtpEmail, smtpPassword, smtpHost)

	err := smtp.SendMail(smtpHost+":"+smtpPort, auth, smtpEmail, adminEmails, fullMessage)
	if err != nil {
		log.Println("Ошибка отправки письма:", err)
	} else {
		log.Printf("Уведомление о сервере %s успешно отправлено на почту!\n", alert.ServerName)
	}
}
