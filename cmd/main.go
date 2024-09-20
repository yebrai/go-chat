package main

import (
	"GoChat/internal/server"
	"log"
	"net/http"
)

func main() {
	srv := server.New()
	log.Println("Servidor escuchando en http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", srv))
}
