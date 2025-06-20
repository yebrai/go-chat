package main

import (
	"log"
	"net/http"
	"time" // For potential future config like timeouts

	"chat-app/internal/infrastructure/api" // Import the new router package
	// server "GoChat/internal/server" // Old import, remove
)

func main() {
	// Configuration values - these should ideally come from environment variables or config files.
	cfg := api.Config{
		PostgresDSN:        "user=postgres password=secret dbname=chat_app_db host=localhost port=5432 sslmode=disable",
		RedisURL:           "redis://localhost:6379/0",
		JWTSecretKey:       "your-very-secure-and-long-secret-key-for-hs256", // Replace with a strong, random key
		JWTAccessTokenTTL:  15 * time.Minute,                                 // Standard access token TTL
		JWTRefreshTokenTTL: 7 * 24 * time.Hour,                               // 7 days for refresh token
		JWTIssuer:          "chat-app.example.com",                           // Your app's identifier
		JWTAudience:        "chat-app.users",                                 // Intended audience
	}

	// Setup the router using the new SetupRouter function that returns a gorilla/mux router
	router := api.SetupRouter(cfg)

	// Define server parameters
	serverAddr := ":8080"
	httpServer := &http.Server{
		Handler:      router, // Use the gorilla/mux router
		Addr:         serverAddr,
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
		IdleTimeout:  60 * time.Second,
		// TODO: Add TLS configuration for HTTPS in production
	}

	log.Printf("Server starting on %s", serverAddr)

	// Start the server
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Could not listen on %s: %v\n", serverAddr, err)
	}

	log.Println("Server stopped gracefully.")
}
