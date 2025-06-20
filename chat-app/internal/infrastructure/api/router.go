package api

import (
	"context" // Ensure context is imported for service interface definitions
	"database/sql"
	"fmt"
	"log"
	"net/http"
	// "strings" // No longer needed for path parsing with gorilla/mux
	"time"

	chat_app "chat-app/internal/application/chat"
	user_app "chat-app/internal/application/user"
	"chat-app/internal/domain/chat"
	"chat-app/internal/domain/user"
	"chat-app/internal/infrastructure/auth" // Import auth package for JWTService
	"chat-app/internal/infrastructure/persistence"
	"chat-app/internal/infrastructure/websocket"

	"github.com/go-redis/redis/v8"
	"github.com/gorilla/mux" // Import gorilla/mux
	gwebsocket "github.com/gorilla/websocket"
	// _ "github.com/lib/pq" // Driver for postgres, imported in persistence
)

var wsUpgrader = gwebsocket.Upgrader{ // Renamed to avoid conflict with gorilla/mux.vars
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all connections for now.
	},
}

// serveWs handles websocket requests from the peer.
// It now also takes JWTService to authenticate connections via token in query param.
func serveWs(hub *websocket.Hub, jwtService *auth.JWTService, w http.ResponseWriter, r *http.Request) {
	tokenString := r.URL.Query().Get("token")
	if tokenString == "" {
		log.Println("serveWs: Auth token missing in query parameters")
		http.Error(w, "Auth token required", http.StatusUnauthorized)
		return
	}

	claims, err := jwtService.ValidateToken(tokenString)
	if err != nil {
		log.Printf("serveWs: Invalid auth token: %v", err)
		http.Error(w, "Invalid auth token: "+err.Error(), http.StatusUnauthorized)
		return
	}

	if claims.Type != auth.TokenTypeAccess {
		log.Printf("serveWs: Invalid token type for WebSocket: expected '%s', got '%s'", auth.TokenTypeAccess, claims.Type)
		http.Error(w, "Invalid token type: not an access token", http.StatusUnauthorized)
		return
	}

	userID := claims.UserID
	log.Printf("serveWs: WebSocket authentication successful for UserID: %s", userID)

	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("serveWs: Failed to upgrade connection:", err)
		// No http.Error needed as Upgrade writes response on error
		return
	}
	log.Printf("WebSocket connection established for UserID: %s from %s", userID, r.RemoteAddr)

	client := websocket.NewClient(hub, conn, userID)
	hub.Register() <- client
	go client.WritePump()
	go client.ReadPump()
}

// Config struct to hold configuration values
type Config struct {
	PostgresDSN         string
	RedisURL            string
	JWTSecretKey        string
	JWTAccessTokenTTL   time.Duration
	JWTRefreshTokenTTL  time.Duration
	JWTIssuer           string
	JWTAudience         string
}

// SetupRouter configures the HTTP routes for the application using gorilla/mux.
// It now returns a *mux.Router.
func SetupRouter(cfg Config) *mux.Router {
	router := mux.NewRouter()

	// Apply global middlewares
	router.Use(LoggingMiddleware)
	router.Use(CORSMiddleware)

	// Initialize Database Connection
	db, err := persistence.NewPostgresDB(cfg.PostgresDSN)
	if err != nil {
		log.Fatalf("Failed to connect to PostgreSQL: %v", err)
	}
	log.Println("Successfully connected to PostgreSQL.")

	// Initialize Redis Client
	redisClient, err := persistence.NewRedisClient(cfg.RedisURL)
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	log.Println("Successfully connected to Redis.")

	// Initialize Repositories
	postgresUserRepo := persistence.NewPostgresUserRepository(db)
	cachedUserRepo := persistence.NewRedisUserRepository(postgresUserRepo, redisClient, 1*time.Hour)

	roomRepo := persistence.NewPostgresRoomRepository(db)
	messageRepo := persistence.NewPostgresMessageRepository(db)

	// Initialize Services (ensure these service types implement the interfaces expected by handlers)
	userServiceInstance := user.NewUserService(cachedUserRepo)
	chatServiceInstance := chat.NewChatService(roomRepo, messageRepo)

	// Initialize WebSocket Hub
	hub := websocket.NewHub(/* chatServiceInstance */) // Pass chatService if Hub persists messages
	go hub.Run()
	log.Println("WebSocket Hub initialized and running.")

	// Initialize JWTService
	jwtService, err := auth.NewJWTService(
		cfg.JWTSecretKey,
		cfg.JWTAccessTokenTTL,
		cfg.JWTRefreshTokenTTL,
		cfg.JWTIssuer,
		cfg.JWTAudience,
	)
	if err != nil {
		log.Fatalf("Failed to initialize JWTService: %v", err)
	}
	log.Println("JWTService initialized.")

	// Initialize Handlers
	// UserHandler now needs JWTService for token generation in Login and RefreshToken
	userHandler := user_app.NewUserHandler(userServiceInstance, jwtService)
	chatHandler := chat_app.NewChatHandler(chatServiceInstance)

	// Static file serving - gorilla/mux can also serve static files
	// Example: router.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("./web/static"))))
	// For simplicity, if your existing Handle and HandleFunc for static files work with net/http.ServeMux behavior,
	// you might need to adapt how they are registered or use mux.PathPrefix.
	// The previous setup used http.ServeMux's Handle for "/" and "/assets/".
	// With gorilla/mux, this is typically done with PathPrefix.
	router.PathPrefix("/assets/").Handler(http.StripPrefix("/assets/", http.FileServer(http.Dir("./web/assets"))))
	// For the root path, ensure it doesn't catch API routes. Usually, API routes are prefixed.
	// If "/" is for a SPA, it should be the last route or carefully configured.
	// For now, let's assume web/static/index.html is the entry point.
	router.Path("/").Handler(http.FileServer(http.Dir("./web/static")))


	// WebSocket handler route
	router.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		serveWs(hub, jwtService, w, r) // Pass jwtService to serveWs
	})

	// API v1 Subrouter - useful for versioning and applying specific middleware
	apiV1 := router.PathPrefix("/api/v1").Subrouter()

	// Public User Routes
	apiV1.HandleFunc("/users/register", userHandler.Register).Methods("POST")
	apiV1.HandleFunc("/users/login", userHandler.Login).Methods("POST")
	apiV1.HandleFunc("/auth/refresh", userHandler.RefreshToken).Methods("POST") // Refresh token route

	// Authenticated User Routes
	// Create an instance of the AuthMiddleware using the factory
	authMw := AuthMiddlewareFactory(jwtService)

	authRoutes := apiV1.PathPrefix("").Subrouter()
	authRoutes.Use(authMw) // Apply the actual auth middleware instance

	authRoutes.HandleFunc("/users/profile", userHandler.UpdateUserProfile).Methods("PUT")
	// Note: GetUserProfile for self can be /users/me or similar.
	// For now, /users/{id} is public or auth is checked inside handler based on {id} vs authenticated user.
	// To make /users/{id} authenticated:
	// authRoutes.HandleFunc("/users/{id}", userHandler.GetUserProfile).Methods("GET")
	// If it can be public for some, and private for 'me', logic inside handler is needed or separate routes.
	// Let's make fetching any user profile public for now for simplicity, but updates are auth'd.
	apiV1.HandleFunc("/users/{id}", userHandler.GetUserProfile).Methods("GET")


	// Authenticated Chat Routes
	authRoutes.HandleFunc("/rooms", chatHandler.CreateRoom).Methods("POST")
	authRoutes.HandleFunc("/users/me/rooms", chatHandler.ListUserRooms).Methods("GET")
	// GetRoomMessages might need auth to check if user is part of the room, or if room is public
	// For now, let's assume it's an authenticated route.
	authRoutes.HandleFunc("/rooms/{id}/messages", chatHandler.GetRoomMessages).Methods("GET")

	// Public Chat Routes
	apiV1.HandleFunc("/rooms", chatHandler.ListPublicRooms).Methods("GET")


	log.Println("HTTP Server configured with Gorilla Mux router, API routes, and WebSocket endpoint.")
	return router
}

// Ensure context is imported by domain services or here if interfaces are defined here.
var _ context.Context
var _ = redis.Nil // Example usage to satisfy import if redis not used elsewhere in this file directly.
var _ = sql.ErrNoRows // Example usage for sql.
