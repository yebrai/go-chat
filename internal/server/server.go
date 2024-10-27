package server

import (
	"GoChat/internal/websocket"
	"net/http"
)

// New creates and configures a new HTTP server
func New() *http.ServeMux {
	mux := http.NewServeMux()
	// Sirve archivos estáticos directamente desde la carpeta "static"
	mux.Handle("/", http.FileServer(http.Dir("./web/static")))
	// Configura la ruta correcta para los archivos en "assets/js"
	mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir("./web/assets"))))
	// WebSocket handler
	mux.HandleFunc("/ws", websocket.HandleConnections)
	// Manejo de mensajes concurrentes para WebSocket
	go websocket.HandleMessages()
	return mux
}
