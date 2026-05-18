package main

import (
	"context"
	"log"

	"github.com/redis/go-redis/v9"
)

var RDB *redis.Client
var Ctx = context.Background()

func InitRedis() {
	RDB = redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,
	})

	_, err := RDB.Ping(Ctx).Result()
	if err != nil {
		log.Fatal("Ошибка подключения к Redis: ", err)
	}
	log.Println("Успешное подключение к Redis Pub/Sub!")
}
