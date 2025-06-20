package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	// "strings" // Not currently used, but could be for more advanced query param validation.

	"chat-app/internal/cache"
	"chat-app/internal/websocket" // Importing local websocket package

	gwebsocket "github.com/gorilla/websocket" // Aliased to avoid conflict with local 'websocket'.
)

// upgrader is a package-level variable that configures the WebSocket connection upgrader.
// It specifies buffer sizes and a CheckOrigin function to control cross-origin requests.
var upgrader = gwebsocket.Upgrader{
	ReadBufferSize:  1024, // Size of the underlying buffer for reading from the connection.
	WriteBufferSize: 1024, // Size of the underlying buffer for writing to the connection.
	CheckOrigin: func(r *http.Request) bool {
		// TODO: In a production environment, validate the request's origin.
		// For example, allow requests only from your frontend's domain:
		// origin := r.Header.Get("Origin")
		// return origin == "https://yourfrontend.com"
		origin := r.Header.Get("Origin") // Get origin for logging.
		log.Printf("HTTP_HANDLER: Upgrading WebSocket connection. Origin: '%s'. Allowing for development.", origin)
		return true // Allow all origins for development purposes.
	},
}

// ChatHandler handles HTTP requests related to chat functionalities, primarily WebSocket connections
// and potentially other auxiliary endpoints like fetching room statistics.
type ChatHandler struct {
	hub         *websocket.Hub     // Reference to the central WebSocket Hub.
	redisClient *cache.RedisClient // Reference to the Redis client for cache/store operations.
}

// NewChatHandler creates and returns a new ChatHandler instance.
// It requires a non-nil Hub and RedisClient.
func NewChatHandler(hub *websocket.Hub, redisClient *cache.RedisClient) *ChatHandler {
	if hub == nil {
		log.Fatal("HTTP_HANDLER_FATAL: Hub cannot be nil in NewChatHandler")
	}
	if redisClient == nil {
		log.Fatal("HTTP_HANDLER_FATAL: RedisClient cannot be nil in NewChatHandler")
	}
	return &ChatHandler{
		hub:         hub,
		redisClient: redisClient,
	}
}

// ServeWs handles incoming WebSocket connection requests.
// It expects 'username' and 'roomID' as query parameters in the request URL.
// If valid, it upgrades the HTTP connection to a WebSocket connection, creates a new
// Client instance, registers it with the Hub, and starts its read/write pumps.
func (ch *ChatHandler) ServeWs(w http.ResponseWriter, r *http.Request) {
	// Extract username and initial roomID from query parameters.
	username := r.URL.Query().Get("username")
	roomID := r.URL.Query().Get("roomID") // Client intends to join this room initially.

	// Validate required query parameters.
	if username == "" {
		log.Println("HTTP_HANDLER_WARN: ServeWs - Username missing from query parameters.")
		http.Error(w, "Query parameter 'username' is required.", http.StatusBadRequest)
		return
	}
	if roomID == "" {
		log.Println("HTTP_HANDLER_WARN: ServeWs - RoomID missing from query parameters.")
		http.Error(w, "Query parameter 'roomID' is required for initial room join.", http.StatusBadRequest)
		return
	}

	// Upgrade the HTTP connection to a WebSocket connection.
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		// upgrader.Upgrade automatically sends an HTTP error response on failure.
		log.Printf("HTTP_HANDLER_ERROR: Failed to upgrade WebSocket connection for user '%s', room '%s': %v", username, roomID, err)
		return
	}
	log.Printf("HTTP_HANDLER: WebSocket connection successfully upgraded for user '%s', initial room '%s'.", username, roomID)

	// Create a new WebSocket client instance.
	client := websocket.NewClient(ch.hub, conn, username, roomID)

	// Register the new client with the Hub.
	// The Hub's RegisterClient method handles sending the client to the internal register channel.
	ch.hub.RegisterClient(client)

	// Start the client's read and write pumps as separate goroutines.
	// These methods handle the lifecycle of message exchange for the client.
	go client.WritePump()
	go client.ReadPump()

	log.Printf("HTTP_HANDLER: Client '%s' registered with Hub; read/write pumps started for room '%s'.", username, roomID)
}

// GetRoomStatsHTTP handles HTTP GET requests for retrieving statistics of a specific chat room.
// It expects a 'roomID' as a query parameter.
// Responds with a JSON containing active user count and total message count for the room.
func (ch *ChatHandler) GetRoomStatsHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		log.Printf("HTTP_HANDLER_WARN: GetRoomStatsHTTP - Invalid method: %s. Only GET allowed.", r.Method)
		http.Error(w, "Only GET method is allowed for this endpoint.", http.StatusMethodNotAllowed)
		return
	}

	roomID := r.URL.Query().Get("roomID")
	if roomID == "" {
		log.Println("HTTP_HANDLER_WARN: GetRoomStatsHTTP - RoomID missing from query parameters.")
		http.Error(w, "Query parameter 'roomID' is required.", http.StatusBadRequest)
		return
	}

	log.Printf("HTTP_HANDLER: Received request for room stats: RoomID '%s'.", roomID)
	ctx := r.Context() // Use request context for Redis operation.
	stats, err := ch.redisClient.GetRoomStats(ctx, roomID)
	if err != nil {
		log.Printf("HTTP_HANDLER_ERROR: Fetching stats for room '%s': %v", roomID, err)
		// Avoid exposing detailed internal errors to the client.
		http.Error(w, "Failed to fetch room statistics. Please try again later.", http.StatusInternalServerError)
		return
	}

	// Define an anonymous struct for a clear JSON response structure.
	responsePayload := struct {
		RoomID       string `json:"room_id"`
		ActiveUsers  int64  `json:"active_users"`
		MessageCount int64  `json:"message_count"`
	}{
		RoomID:       roomID,
		ActiveUsers:  stats["active_users"],
		MessageCount: stats["message_count"],
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK) // Explicitly set StatusOK.
	if err := json.NewEncoder(w).Encode(responsePayload); err != nil {
		// This error occurs if writing to ResponseWriter fails after headers are set.
		log.Printf("HTTP_HANDLER_ERROR: Encoding room stats JSON response for room '%s': %v", roomID, err)
		// Cannot send http.Error here as headers/status might have been written.
	}
	log.Printf("HTTP_HANDLER: Successfully sent stats for room '%s'. Users: %d, Msgs: %d", roomID, responsePayload.ActiveUsers, responsePayload.MessageCount)
}
