package main

import (
	"golang.org/x/net/websocket"
	"log"
	"net/http"
	"websocket-cluster/handler"
)

func main() {
	http.Handle("/", websocket.Handler(handler.HandleChannel))

	if err := http.ListenAndServe(":5678", nil); err != nil {
		log.Fatal("ListenAndServe:", err)
	}
}
