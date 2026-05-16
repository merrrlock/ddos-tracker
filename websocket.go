package main

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func WsAlertsHandler(c *gin.Context) {
	ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Println("Ошибка WebSocket upgrade:", err)
		return
	}
	defer ws.Close()

	log.Println("подключение по WebSocket успешно")

	pubsub := RDB.Subscribe(Ctx, "alerts_channel")
	defer pubsub.Close()

	ch := pubsub.Channel()

	for msg := range ch {
		err := ws.WriteMessage(websocket.TextMessage, []byte(msg.Payload))
		if err != nil {
			log.Println("Клиент отключился или произошла ошибка записи:", err)
			break
		}
	}
}
