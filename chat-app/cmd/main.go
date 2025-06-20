package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/yebrai/go-chat/internal/cache"
	"github.com/yebrai/go-chat/internal/handlers"
	"github.com/yebrai/go-chat/internal/websocket"
)

func main() {
	// Application starting point.
	log.Println("MAIN: Starting Simple Go Chat Application...")

	// --- Configuration Setup ---
	// Retrieve Redis URL from environment variable or use default.
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379/0" // Default Redis connection URL.
		log.Printf("MAIN_CONFIG: REDIS_URL not set in environment, using default: %s", redisURL)
	} else {
		log.Printf("MAIN_CONFIG: Using REDIS_URL from environment: %s", redisURL)
	}

	// Retrieve Port from environment variable or use default.
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080" // Default HTTP port.
		log.Printf("MAIN_CONFIG: PORT not set in environment, using default: %s", port)
	} else {
		log.Printf("MAIN_CONFIG: Using PORT from environment: %s", port)
	}

	// --- Dependency Initialization ---
	// Initialize Redis Client. This is a critical dependency.
	redisClient, err := cache.NewRedisClient(redisURL)
	if err != nil {
		log.Fatalf("MAIN_FATAL: Failed to initialize Redis client at %s: %v", redisURL, err)
	}
	// Ensure Redis client is closed gracefully on application shutdown.
	defer func() {
		log.Println("MAIN: Closing Redis client connection...")
		if err := redisClient.Close(); err != nil {
			log.Printf("MAIN_ERROR: Closing Redis client: %v", err)
		}
		log.Println("MAIN: Redis client connection closed.")
	}()
	log.Println("MAIN: Redis client initialized successfully.")

	// Initialize WebSocket Hub. The Hub requires the Redis client.
	hub := websocket.NewHub(redisClient)
	// Start the Hub's main processing loop as a separate goroutine.
	// This allows the Hub to handle events concurrently with the HTTP server.
	go hub.Run()
	log.Println("MAIN: WebSocket Hub initialized and running in a separate goroutine.")

	// Initialize HTTP Handlers. The ChatHandler requires the Hub and Redis client.
	chatHandler := handlers.NewChatHandler(hub, redisClient)
	log.Println("MAIN: Chat HTTP handler initialized.")

	// --- HTTP Router Setup ---
	// Create a new ServeMux for routing HTTP requests.
	mux := http.NewServeMux()

	// Register the WebSocket connection handler.
	mux.HandleFunc("/ws", chatHandler.ServeWs)
	log.Printf("MAIN_ROUTES: WebSocket endpoint registered at /ws")

	// Register an optional HTTP endpoint for fetching room statistics.
	if chatHandler.GetRoomStatsHTTP != nil { // This check is mostly for robustness if it were truly optional.
		mux.HandleFunc("/api/rooms/stats", chatHandler.GetRoomStatsHTTP)
		log.Printf("MAIN_ROUTES: Room stats API endpoint registered at /api/rooms/stats")
	}

	// Setup static file serving for the frontend assets.
	// Files are served from the "./chat-app/web" directory.
	// For example, a request to "/" will serve "./chat-app/web/index.html".
	// Requests to "/style.css" will serve "./chat-app/web/style.css".
	staticFileServer := http.FileServer(http.Dir("./chat-app/web"))
	mux.Handle("/", staticFileServer)
	log.Printf("MAIN_ROUTES: Static files served from directory ./chat-app/web at root path /")

	// --- HTTP Server Start ---
	serverAddr := ":" + port
	log.Printf("MAIN: HTTP server starting on http://localhost%s ...", serverAddr)

	// Configure the HTTP server.
	httpServer := &http.Server{
		Addr:    serverAddr,
		Handler: mux, // Use the configured ServeMux.
		// Set timeouts to avoid resource exhaustion from slow or malicious clients.
		ReadTimeout:  10 * time.Second,  // Max time for reading the entire request, including body.
		WriteTimeout: 10 * time.Second,  // Max time for writing the response.
		IdleTimeout:  120 * time.Second, // Max time for an idle connection.
	}

	// Start the HTTP server and log any fatal errors.
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("MAIN_FATAL: Could not start HTTP server on %s: %v\n", serverAddr, err)
	}

	// This log message will typically not be reached if ListenAndServe runs indefinitely,
	// unless the server is gracefully shut down (which is not explicitly handled here but http.ErrServerClosed checks for it).
	log.Println("MAIN: Server stopped gracefully.")
}
