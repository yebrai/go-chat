package server

import (
	"GoChat/internal/websocket"
	"net/http"
)

func New() *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.Dir("./web/static")))
	mux.HandleFunc("/ws", websocket.HandleConnections)
	go websocket.HandleMessages()
	return mux
}
