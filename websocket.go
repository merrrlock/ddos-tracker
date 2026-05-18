package main

import (
	"log"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

var (
	activeClients = make(map[*websocket.Conn]bool)
	clientsMutex  sync.Mutex
)

func handleConnections(c *gin.Context) {
	ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("Ошибка Upgrade: %v", err)
		return
	}

	clientsMutex.Lock()
	activeClients[ws] = true
	clientsMutex.Unlock()

	log.Println("📱 Новый мобильный клиент успешно подключен!")

	defer func() {
		clientsMutex.Lock()
		delete(activeClients, ws)
		clientsMutex.Unlock()
		ws.Close()
		log.Println("📱 Клиент отключился")
	}()

	for {
		_, _, err := ws.ReadMessage()
		if err != nil {
			break
		}
	}
}

func BroadcastAlert(jsonMessage []byte) {
	clientsMutex.Lock()
	defer clientsMutex.Unlock()

	for client := range activeClients {
		err := client.WriteMessage(websocket.TextMessage, jsonMessage)
		if err != nil {
			log.Printf("Ошибка отправки клиенту: %v", err)
			client.Close()
			delete(activeClients, client)
		}
	}
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
