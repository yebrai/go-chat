package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"chat-app/internal/cache"
)

const (
	// defaultRoomUsersSetTTL is the default TTL for a room's user set in Redis.
	// This helps in cleaning up user sets for inactive or empty rooms.
	defaultRoomUsersSetTTL = 2 * time.Hour

	// defaultRecentMessagesListTTL is the default TTL for a room's recent messages list in Redis.
	// This manages memory for message history of inactive rooms.
	defaultRecentMessagesListTTL = 24 * time.Hour

	// maxRecentMessagesToStore is the maximum number of recent messages stored per room in Redis.
	maxRecentMessagesToStore = 50

	// maxRecentMessagesToSend is the maximum number of recent messages sent to a client upon joining a room.
	maxRecentMessagesToSend = 20 // Can be same or less than maxRecentMessagesToStore
)

// Hub maintains the set of active clients, manages chat rooms,
// and broadcasts messages to the appropriate clients. It uses a RedisClient
// for persisting certain data like recent messages and room user lists.
type Hub struct {
	clients      map[*Client]bool            // Actively connected clients.
	rooms        map[string]map[*Client]bool // Map of roomID to a set of clients in that room.
	register     chan *Client                // Channel for clients wishing to register.
	unregister   chan *Client                // Channel for clients wishing to unregister.
	routeMessage chan *Message               // Channel for messages from clients to be processed by the Hub.
	redisClient  *cache.RedisClient          // Client for interacting with Redis cache/store.
	mu           sync.RWMutex                // Mutex to protect concurrent access to `clients` and `rooms` maps.
}

// NewHub creates and returns a new Hub instance.
// It requires a `cache.RedisClient` for its operations.
// The Hub's `Run` method should be started as a goroutine after creation.
func NewHub(redisClient *cache.RedisClient) *Hub {
	if redisClient == nil {
		// This is a critical dependency, so panic or fatal log is appropriate.
		log.Fatal("HUB: Redis client cannot be nil for Hub initialization")
	}
	h := &Hub{
		clients:      make(map[*Client]bool),
		rooms:        make(map[string]map[*Client]bool),
		register:     make(chan *Client), // Unbuffered, registration should be handled promptly.
		unregister:   make(chan *Client), // Unbuffered.
		routeMessage: make(chan *Message, 256), // Buffered to handle bursts of messages.
		redisClient:  redisClient,
	}
	log.Println("HUB: Hub instance created successfully.")
	return h
}

// RegisterClient provides a thread-safe way for external components (e.g., HTTP handlers)
// to register a new client with the Hub. It sends the client to the Hub's internal `register` channel.
func (h *Hub) RegisterClient(client *Client) {
	if client == nil {
		log.Println("HUB_ERROR: Attempted to register a nil client.")
		return
	}
	log.Printf("HUB: Queuing client %s for registration.", client.username)
	select {
	case h.register <- client:
		log.Printf("HUB: Client %s successfully queued for registration.", client.username)
	case <-time.After(2 * time.Second): // Timeout to prevent blocking indefinitely if Run() isn't active.
		log.Printf("HUB_ERROR: Registration timeout for client %s. Hub may not be running or register channel full.", client.username)
		close(client.send) // Close client's send channel to signal error and stop its writePump.
		_ = client.conn.Close()    // Close WebSocket connection.
	}
}

// Run starts the Hub's main event processing loop.
// It listens on its channels for client registrations, unregistrations,
// and incoming messages, and processes them accordingly.
// This method should be run as a goroutine.
func (h *Hub) Run() {
	log.Println("HUB: Starting event loop...")
	for {
		select {
		case client := <-h.register:
			h.handleClientRegistration(client)
		case client := <-h.unregister:
			h.handleClientUnregistration(client)
		case message := <-h.routeMessage:
			h.handleIncomingMessage(message)
		}
	}
}

// handleClientRegistration processes a new client registration.
// It adds the client to the global client list, adds the user to the global Redis set,
// broadcasts the updated global user count, and then attempts to join the client
// to their specified initial room.
func (h *Hub) handleClientRegistration(client *Client) {
	h.mu.Lock()
	h.clients[client] = true
	h.mu.Unlock()
	log.Printf("HUB: Client '%s' (room: '%s') registered with Hub.", client.username, client.currentRoomID)

	// Add user to global set in Redis.
	if err := h.redisClient.AddUserToGlobalSet(context.Background(), client.username); err != nil {
		log.Printf("HUB_ERROR: Adding user '%s' to global Redis set: %v", client.username, err)
	}
	h.broadcastGlobalUserCount() // Inform all clients about the new global user count.

	// Handle initial room join for the client.
	if client.currentRoomID != "" {
		h.handleClientJoinRoom(client, client.currentRoomID)
	} else {
		log.Printf("HUB: Client '%s' connected without specifying an initial room.", client.username)
		// Optionally, send a welcome message or instructions to the client.
		// client.send <- &Message{Type: SystemMessage, Content: "Welcome! Please join a room." ...}
	}
}

// handleClientUnregistration processes a client unregistration.
// It ensures the client is removed from any room they were in,
// removes the user from global Redis sets, updates and broadcasts global user count,
// closes the client's send channel, and removes the client from the Hub's active list.
func (h *Hub) handleClientUnregistration(client *Client) {
	h.mu.Lock()
	isRegistered := h.clients[client] // Check if client is actually in the map.
	if isRegistered {
		delete(h.clients, client)
		close(client.send) // Important: Close the send channel to stop writePump and signal cleanup.
		log.Printf("HUB: Client '%s' unregistered from Hub.", client.username)
	}
	h.mu.Unlock() // Unlock before potentially long-running or lock-acquiring operations.

	if isRegistered { // Only proceed if client was part of the hub.
		// Ensure client leaves their current room. `true` indicates a full disconnect.
		if client.currentRoomID != "" {
			h.handleClientLeaveRoom(client, client.currentRoomID, true)
		}

		// Remove user from global set in Redis.
		if err := h.redisClient.RemoveUserFromGlobalSet(context.Background(), client.username); err != nil {
			log.Printf("HUB_ERROR: Removing user '%s' from global Redis set: %v", client.username, err)
		}
		h.broadcastGlobalUserCount() // Update global user count for all remaining clients.
	}
}

// handleIncomingMessage processes a message received from a client via the `routeMessage` channel.
// It routes the message based on its `Type`.
func (h *Hub) handleIncomingMessage(msg *Message) {
	log.Printf("HUB: Routing message type '%s' from user '%s' for room '%s'. Content: '%.50s'", msg.Type, msg.Username, msg.RoomID, msg.Content)

	// Server should always set the timestamp for messages it processes or broadcasts.
	msg.Timestamp = time.Now().UTC()

	client := h.findClientByUsername(msg.Username) // Find the client instance.
	if client == nil && msg.Type != "" { // Allow empty type for potential internal messages? No, client messages must have type.
		log.Printf("HUB_WARN: Message received from unknown or unregistered user '%s'. Type: '%s'. Discarding.", msg.Username, msg.Type)
		return
	}

	switch msg.Type {
	case TextMessageType:
		if msg.RoomID == "" || msg.Username == "" {
			log.Printf("HUB_WARN: Text message missing RoomID ('%s') or Username ('%s'). Discarding.", msg.RoomID, msg.Username)
			if client != nil {
				client.send <- &Message{Type: ErrorMessageType, Content: "Your message could not be sent: RoomID or Username was missing.", Timestamp: time.Now().UTC()}
			}
			return
		}
		msg.System = false // Ensure it's marked as a user-generated message.

		messageJSON, err := json.Marshal(msg) // Serialize the websocket.Message for storage.
		if err != nil {
			log.Printf("HUB_ERROR: Marshalling text message to JSON for Redis (user '%s', room '%s'): %v", msg.Username, msg.RoomID, err)
			return // Don't proceed if we can't store it.
		}

		// Persist message to Redis recent messages list.
		err = h.redisClient.AddRecentMessage(context.Background(), msg.RoomID, string(messageJSON), maxRecentMessagesToStore, 0) // Use default TTL from cache pkg
		if err != nil {
			log.Printf("HUB_ERROR: Adding message to Redis for room '%s' by user '%s': %v", msg.RoomID, msg.Username, err)
		}

		// Increment room message counter.
		if _, err = h.redisClient.IncrementMessageCounter(context.Background(), msg.RoomID); err != nil {
			log.Printf("HUB_ERROR: Incrementing message counter for room '%s': %v", msg.RoomID, err)
		}

		h.broadcastToRoom(msg)           // Broadcast the live message.
		h.broadcastRoomStats(msg.RoomID) // Update and broadcast room stats (e.g., new message count).

	case JoinRoomMessageType:
		var joinData JoinRoomData
		// Assuming client sends JoinRoomData as JSON string in msg.Content
		if err := json.Unmarshal([]byte(msg.Content), &joinData); err != nil {
			log.Printf("HUB_ERROR: Unmarshalling join_room data from user '%s': %v. Raw content: '%s'", msg.Username, err, msg.Content)
			if client != nil {
				client.send <- &Message{Type: ErrorMessageType, Content: "Invalid join room request format.", Timestamp: time.Now().UTC()}
			}
			return
		}
		if client == nil { // Should have been caught earlier, but double check.
			log.Printf("HUB_ERROR: Client '%s' not found for join_room request to room '%s'.", msg.Username, joinData.RoomID)
			return
		}
		if joinData.RoomID == "" {
			log.Printf("HUB_WARN: User '%s' attempted to join an empty RoomID.", msg.Username)
			client.send <- &Message{Type: ErrorMessageType, Content: "Cannot join an empty RoomID.", Timestamp: time.Now().UTC()}
			return
		}

		if client.currentRoomID != "" && client.currentRoomID != joinData.RoomID {
			log.Printf("HUB: Client '%s' leaving room '%s' to join '%s'.", client.username, client.currentRoomID, joinData.RoomID)
			h.handleClientLeaveRoom(client, client.currentRoomID, false) // `false` means not a full disconnect.
		}
		h.handleClientJoinRoom(client, joinData.RoomID)

	case LeaveRoomMessageType: // Client explicitly wants to leave current room.
		if client != nil && client.currentRoomID != "" {
			log.Printf("HUB: Client '%s' leaving room '%s' by request.", client.username, client.currentRoomID)
			h.handleClientLeaveRoom(client, client.currentRoomID, false)
			client.currentRoomID = "" // Clear client's current room as they've left it.
			// Optionally send confirmation to client: client.send <- &Message{Type: SystemMessage, Content: "You have left the room." ...}
		}

	case UserTypingMessageType:
		if msg.RoomID != "" && msg.Username != "" {
			// Content should be "start" or "stop". This is broadcast to others in the room.
			log.Printf("HUB: User '%s' typing status '%s' in room '%s'.", msg.Username, msg.Content, msg.RoomID)
			h.broadcastToRoom(msg) // The message itself contains all necessary info (type, username, room, content).
		}

	case RequestStatsType:
		if client == nil { return } // Should not happen if check at start of function is good.
		targetRoomID := msg.RoomID
		if targetRoomID == "" { // If client requests stats for their current room without specifying
			targetRoomID = client.currentRoomID
		}
		if targetRoomID == "" {
			log.Printf("HUB_WARN: User '%s' requested stats for an unspecified room.", msg.Username)
			client.send <- &Message{Type: ErrorMessageType, Content: "RoomID required for stats request.", Timestamp: time.Now().UTC()}
			return
		}

		stats, err := h.redisClient.GetRoomStats(context.Background(), targetRoomID)
		if err != nil {
			log.Printf("HUB_ERROR: Getting room stats for '%s' (requested by '%s'): %v", targetRoomID, msg.Username, err)
			client.send <- &Message{Type: ErrorMessageType, Content: "Failed to get room stats for " + targetRoomID, RoomID: targetRoomID, Timestamp: time.Now().UTC()}
			return
		}
		log.Printf("HUB: Sending stats for room '%s' to user '%s'. Users: %d, Msgs: %d", targetRoomID, msg.Username, stats["active_users"], stats["message_count"])
		client.send <- &Message{
			Type:   RoomStatsUpdateType, // Send as a stats update.
			RoomID: targetRoomID,
			Data: RoomStatsPayload{
				RoomID:       targetRoomID,
				ActiveUsers:  stats["active_users"],
				MessageCount: stats["message_count"],
			},
			Timestamp: time.Now().UTC(),
			System:    true, // Stats are system-generated info.
		}

	default:
		log.Printf("HUB_WARN: Unknown message type '%s' received from user '%s'. Discarding.", msg.Type, msg.Username)
		if client != nil {
			 errorMsg := &Message{
				Type: ErrorMessageType,
				Content: fmt.Sprintf("Unknown message type received: %s", msg.Type),
				Timestamp: time.Now().UTC(),
			 }
			 client.send <- errorMsg
		}
	}
}

// handleClientJoinRoom manages adding a client to a specific room.
// It updates Hub's internal state, Redis store, and broadcasts relevant updates.
func (h *Hub) handleClientJoinRoom(client *Client, roomID string) {
	if roomID == "" {
		log.Printf("HUB_WARN: Client '%s' attempted to join an empty/invalid roomID. Aborting join.", client.username)
		client.send <- &Message{Type: ErrorMessageType, Content: "Invalid RoomID. Cannot join.", Timestamp: time.Now().UTC()}
		return
	}
	// If client was in another room, they should be removed by caller (e.g. handleIncomingMessage for JoinRoomType)
	// This function focuses on the "join" part.

	log.Printf("HUB: Client '%s' joining room '%s'.", client.username, roomID)

	h.mu.Lock()
	if _, ok := h.rooms[roomID]; !ok {
		h.rooms[roomID] = make(map[*Client]bool)
		log.Printf("HUB: Room '%s' created dynamically in Hub.", roomID)
	}
	h.rooms[roomID][client] = true
	client.currentRoomID = roomID // Critical: Update client's state.
	h.mu.Unlock()

	// Add user to Redis set for the room with a TTL.
	if err := h.redisClient.AddActiveUserToRoom(context.Background(), roomID, client.username, 0); err != nil { // Use default TTL from cache pkg
		log.Printf("HUB_ERROR: Adding user '%s' to Redis room set '%s': %v", client.username, roomID, err)
	}

	// Send recent messages to the newly joined client.
	recentMsgJSONs, err := h.redisClient.GetRecentMessages(context.Background(), roomID, maxRecentMessagesToSend)
	if err != nil {
		log.Printf("HUB_ERROR: Getting recent messages for room '%s': %v", roomID, err)
	} else if len(recentMsgJSONs) > 0 {
		client.send <- &Message{
			Type:      RecentMessagesType,
			RoomID:    roomID,
			Data:      RecentMessagesPayload{RoomID: roomID, Messages: recentMsgJSONs},
			Timestamp: time.Now().UTC(),
			System:    true,
		}
		log.Printf("HUB: Sent %d recent messages to user '%s' for room '%s'.", len(recentMsgJSONs), client.username, roomID)
	} else {
		log.Printf("HUB: No recent messages in room '%s' for user '%s'.", roomID, client.username)
	}

	// Broadcast updates to all clients in the room.
	h.broadcastSystemMessageToRoom(roomID, fmt.Sprintf("User '%s' joined the room.", client.username), client.username, UserJoinedMessageType)
	h.broadcastUserList(roomID)  // Send updated user list to everyone in the room.
	h.broadcastRoomStats(roomID) // Send updated room stats to everyone in the room.
}

// handleClientLeaveRoom manages removing a client from a specific room.
// `isDisconnect` is true if the client is fully disconnecting from the Hub.
// It updates Hub's internal state, Redis store, and broadcasts relevant updates.
func (h *Hub) handleClientLeaveRoom(client *Client, roomID string, isDisconnect bool) {
	if roomID == "" {
		// Client might not be in any room if currentRoomID is empty.
		// log.Printf("HUB: Client '%s' attempted to leave an empty roomID. isDisconnect: %t", client.username, isDisconnect)
		return
	}
	log.Printf("HUB: Client '%s' leaving room '%s'. Disconnecting: %t", client.username, roomID, isDisconnect)

	var clientWasInRoomMap bool
	h.mu.Lock()
	roomClients, roomExistsInHub := h.rooms[roomID]
	if roomExistsInHub {
		if _, clientFound := roomClients[client]; clientFound {
			clientWasInRoomMap = true
			delete(h.rooms[roomID], client)
			if len(h.rooms[roomID]) == 0 {
				log.Printf("HUB: Room '%s' is now empty, removing from Hub's active room list.", roomID)
				delete(h.rooms, roomID) // Clean up empty room from Hub's map.
			}
		}
	}
	h.mu.Unlock()

	if clientWasInRoomMap { // Only if client was actually removed from the Hub's room map.
		// Remove user from Redis set for the room.
		if err := h.redisClient.RemoveActiveUserFromRoom(context.Background(), roomID, client.username); err != nil {
			log.Printf("HUB_ERROR: Removing user '%s' from Redis room set '%s': %v", client.username, roomID, err)
		}

		// Broadcast updates to remaining clients in the room.
		h.broadcastSystemMessageToRoom(roomID, fmt.Sprintf("User '%s' left the room.", client.username), client.username, UserLeftMessageType)
		h.broadcastUserList(roomID)
		h.broadcastRoomStats(roomID)
	} else {
		log.Printf("HUB_WARN: Client '%s' was not found in Hub's map for room '%s' during leave process.", client.username, roomID)
	}

	// If client is just leaving the room (not disconnecting from WebSocket), clear their current room.
	if !isDisconnect {
		client.currentRoomID = "" // Important: Update client's state.
	}
}

// broadcastToRoom sends a message to all clients currently in the specified room.
// It intelligently skips sending certain self-generated messages (like typing notifications)
// back to the originator.
func (h *Hub) broadcastToRoom(message *Message) {
	if message.RoomID == "" {
		log.Printf("HUB_WARN: Attempted to broadcast message with empty RoomID: Type '%s', User '%s'", message.Type, message.Username)
		return
	}

	h.mu.RLock() // Read-lock to safely access h.rooms and then individual room's client map.
	roomClientsMap, roomExists := h.rooms[message.RoomID]
	if !roomExists {
		h.mu.RUnlock()
		log.Printf("HUB_WARN: Room '%s' not found for broadcasting message type '%s'.", message.RoomID, message.Type)
		return
	}

	// Create a list of clients to send to, to avoid holding lock while sending,
	// which can be slow and cause contention.
	clientsToSend := make([]*Client, 0, len(roomClientsMap))
	for c := range roomClientsMap {
		// Don't send "user_typing" message back to the user who is typing.
		// For text messages, server broadcasts to all including sender (client can identify own messages).
		if message.Type == UserTypingMessageType && c.username == message.Username {
			continue
		}
		clientsToSend = append(clientsToSend, c)
	}
	h.mu.RUnlock() // Release lock before starting send operations.

	log.Printf("HUB: Broadcasting message type '%s' to %d clients in room '%s'.", message.Type, len(clientsToSend), message.RoomID)
	for _, c := range clientsToSend {
		select {
		case c.send <- message:
		default:
			// Client's send buffer is full or channel is closed.
			// This indicates the client is slow, stuck, or disconnected.
			// Schedule unregistration to clean up.
			log.Printf("HUB_WARN: Client '%s' send channel full/closed for room '%s' broadcast. Scheduling unregister.", c.username, message.RoomID)
			// Run unregistration in a new goroutine to avoid blocking the broadcast loop.
			go func(cl *Client) { h.unregister <- cl }(c)
		}
	}
}

// broadcastSystemMessageToRoom is a helper to construct and broadcast system messages.
func (h *Hub) broadcastSystemMessageToRoom(roomID, content, relevantUsername string, msgType MessageType) {
	if roomID == "" { return } // Basic validation
	log.Printf("HUB: Broadcasting system message to room '%s': Type '%s', Content '%s', User '%s'", roomID, msgType, content, relevantUsername)
	sysMsg := &Message{
		Type:      msgType,
		Content:   content,
		Username:  relevantUsername, // User who triggered or is relevant to the system event.
		RoomID:    roomID,
		Timestamp: time.Now().UTC(),
		System:    true,
	}
	h.broadcastToRoom(sysMsg)
}

// broadcastUserList fetches the current user list for a room from Redis
// and broadcasts it to all clients in that room.
func (h *Hub) broadcastUserList(roomID string) {
	if roomID == "" { return }
	users, err := h.redisClient.GetActiveUsersInRoom(context.Background(), roomID)
	if err != nil {
		log.Printf("HUB_ERROR: Getting active users for room '%s' to broadcast list: %v", roomID, err)
		return
	}
	log.Printf("HUB: Broadcasting user list for room '%s'. Users: %v", roomID, users)
	userListMsg := &Message{
		Type:      UserListUpdateType,
		RoomID:    roomID,
		Data:      UserListPayload{RoomID: roomID, Users: users},
		Timestamp: time.Now().UTC(),
		System:    true,
	}
	h.broadcastToRoom(userListMsg)
}

// broadcastRoomStats fetches current statistics for a room from Redis
// and broadcasts them to all clients in that room.
func (h *Hub) broadcastRoomStats(roomID string) {
	if roomID == "" { return }
	stats, err := h.redisClient.GetRoomStats(context.Background(), roomID)
	if err != nil {
		log.Printf("HUB_ERROR: Getting room stats for '%s' to broadcast: %v", roomID, err)
		return
	}
	log.Printf("HUB: Broadcasting stats for room '%s'. ActiveUsers: %d, MessageCount: %d", roomID, stats["active_users"], stats["message_count"])
	statsMsg := &Message{
		Type:   RoomStatsUpdateType,
		RoomID: roomID,
		Data: RoomStatsPayload{
			RoomID:       roomID,
			ActiveUsers:  stats["active_users"],
			MessageCount: stats["message_count"],
		},
		Timestamp: time.Now().UTC(),
		System:    true,
	}
	h.broadcastToRoom(statsMsg)
}

// broadcastGlobalUserCount fetches the total number of globally connected users from Redis
// and broadcasts this count to ALL currently connected clients.
func (h *Hub) broadcastGlobalUserCount() {
	count, err := h.redisClient.GetGlobalActiveUserCount(context.Background())
	if err != nil {
		log.Printf("HUB_ERROR: Getting global user count to broadcast: %v", err)
		return
	}
	log.Printf("HUB: Broadcasting global user count: %d", count)
	countMsg := &Message{
		Type:      GlobalUserCountUpdateType,
		Data:      GlobalUserCountPayload{Count: count},
		Timestamp: time.Now().UTC(),
		System:    true,
	}

	h.mu.RLock() // Read-lock h.clients to get current list of all clients.
	// Create a list of clients to send to, to avoid holding lock while sending.
	allClients := make([]*Client, 0, len(h.clients))
	for c := range h.clients {
		allClients = append(allClients, c)
	}
	h.mu.RUnlock()

	for _, c := range allClients {
		select {
		case c.send <- countMsg:
		default:
			log.Printf("HUB_WARN: Client '%s' send channel full for global user count. Scheduling unregister.", c.username)
			go func(cl *Client) { h.unregister <- cl }(c)
		}
	}
}

// findClientByUsername is a helper to find a client pointer by username.
// This can be slow if there are many clients. For a system with a very large
// number of concurrent users, maintaining a separate map[username]*Client
// (also protected by a mutex) would be more efficient for lookups.
// For this educational example, iterating through h.clients is acceptable.
func (h *Hub) findClientByUsername(username string) *Client {
	h.mu.RLock() // Use Read Lock as we are only reading h.clients.
	defer h.mu.RUnlock()
	for client := range h.clients {
		if client.username == username {
			return client
		}
	}
	return nil // Return nil if client not found.
}
