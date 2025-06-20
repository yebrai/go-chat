package websocket

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"chat-app/internal/domain/chat" // For domain.Message
	// "chat-app/internal/domain/chat" // For ChatService if used.
)

// RoomDetail represents a room within the Hub, managing clients in that room.
type RoomDetail struct {
	mu      sync.RWMutex
	clients map[*Client]bool
	roomID  string
}

// Hub maintains the set of active clients and broadcasts messages to the
// clients in specific rooms.
type Hub struct {
	// Registered clients. Key: client pointer, Value: boolean (true)
	clients map[*Client]bool
	// Rooms in the hub. Key: roomID, Value: RoomDetail
	rooms map[string]*RoomDetail
	// Inbound messages from the clients to be routed by the hub.
	routeMessage chan *WebsocketMessage
	// Register requests from the clients.
	register chan *Client
	// Unregister requests from clients.
	unregister chan *Client
	// Mutex for protecting clients and rooms maps
	mu sync.RWMutex

	// chatService chat.ChatService // Optional: For persisting messages, fetching room info etc.
}

// WebsocketMessage defines the structure for messages exchanged over WebSocket.
// This can be expanded with more fields as needed.
type WebsocketMessage struct {
	Type     string          `json:"type"` // e.g., "send_message", "join_room", "leave_room", "user_joined", "user_left", "message_ack"
	Payload  json.RawMessage `json:"payload"` // Flexible payload depending on Type
	ClientID string          `json:"client_id,omitempty"` // Sender's UserID, set by hub or client readPump
	RoomID   string          `json:"room_id,omitempty"`   // Target RoomID, often part of payload for send_message
}

// Payloads for specific message types (examples)
type SendMessagePayload struct {
	RoomID  string `json:"room_id"`
	Content string `json:"content"`
	// Potentially other fields like client-generated message ID for ACK
}

type JoinRoomPayload struct {
	RoomID string `json:"room_id"`
}

type LeaveRoomPayload struct {
	RoomID string `json:"room_id"`
}


func NewHub(/*chatService chat.ChatService*/) *Hub {
	return &Hub{
		routeMessage: make(chan *WebsocketMessage, 256), // Buffered channel
		register:     make(chan *Client),
		unregister:   make(chan *Client),
		clients:      make(map[*Client]bool),
		rooms:        make(map[string]*RoomDetail),
		// chatService:  chatService,
	}
}

func (h *Hub) Run() {
	log.Println("Hub is running...")
	for {
		select {
		case client := <-h.register:
			h.handleRegister(client)
		case client := <-h.unregister:
			h.handleUnregister(client)
		case wsMsg := <-h.routeMessage:
			h.handleRouteMessage(wsMsg)
		}
	}
}

func (h *Hub) handleRegister(client *Client) {
	h.mu.Lock()
	h.clients[client] = true
	h.mu.Unlock()
	log.Printf("Client registered: %s (User ID: %s)", client.conn.RemoteAddr(), client.userID)
	// Optionally, automatically join a default room or wait for a "join_room" message.
}

func (h *Hub) handleUnregister(client *Client) {
	h.mu.Lock()
	if _, ok := h.clients[client]; ok {
		delete(h.clients, client)
		close(client.send) // Close send channel
		// Remove client from all rooms they were in
		for roomID := range client.rooms {
			if room, roomExists := h.rooms[roomID]; roomExists {
				room.mu.Lock()
				delete(room.clients, client)
				if len(room.clients) == 0 { // If room becomes empty, remove it
					// delete(h.rooms, roomID) // Or keep it if rooms are persistent entities
					log.Printf("Room %s is now empty.", roomID)
				}
				room.mu.Unlock()
				// Notify other clients in the room that this user left
				h.broadcastSystemMessage(roomID, client.userID, fmt.Sprintf("User %s left the room.", client.userID), "user_left")

			}
		}
		log.Printf("Client unregistered: %s (User ID: %s)", client.conn.RemoteAddr(), client.userID)
	}
	h.mu.Unlock()
}

func (h *Hub) handleRouteMessage(wsMsg *WebsocketMessage) {
	log.Printf("Routing message type '%s' from client %s", wsMsg.Type, wsMsg.ClientID)
	switch wsMsg.Type {
	case "send_message":
		var payload SendMessagePayload
		if err := json.Unmarshal(wsMsg.Payload, &payload); err != nil {
			log.Printf("Error unmarshalling send_message payload: %v", err)
			// TODO: Send error back to client?
			return
		}
		// For message persistence:
		// 1. Convert WebsocketMessage/SendMessagePayload to domain.Message
		// 2. Call h.chatService.SendMessage(...)
		// 3. Use the persisted domain.Message (with ID, Timestamp from server) for broadcasting.

		// Simplified: directly create a domain.Message like structure for broadcasting
		// In a real app, message persistence via chatService would happen here BEFORE broadcasting.
		// The persisted message (with server-generated ID and timestamp) would be broadcast.
		domainMsg := &chat.Message{
			// ID: This would come from the database after persistence
			RoomID:    payload.RoomID,
			UserID:    wsMsg.ClientID, // Sender's ID
			Content:   payload.Content,
			Timestamp: time.Now().UTC(),
			Type:      chat.TextMessageType, // Assuming text for now, could be part of payload
			// User field would be populated by service if needed for broadcast
		}

		// Convert domain.Message to WebsocketMessage for broadcast
		broadcastPayload, _ := json.Marshal(domainMsg)
		broadcastWsMsg := WebsocketMessage{
			Type: "new_message", // This is what clients will receive
			Payload: broadcastPayload,
			ClientID: wsMsg.ClientID, // Keep sender info
			RoomID: payload.RoomID,
		}
		h.broadcastToRoom(payload.RoomID, &broadcastWsMsg, nil) // nil means don't exclude anyone

	case "join_room":
		var payload JoinRoomPayload
		if err := json.Unmarshal(wsMsg.Payload, &payload); err != nil {
			log.Printf("Error unmarshalling join_room payload: %v", err)
			return
		}
		client := h.findClientByUserID(wsMsg.ClientID) // Need a way to get client pointer from UserID
		if client == nil {
			log.Printf("Error: client %s not found for join_room", wsMsg.ClientID)
			return
		}
		h.handleJoinRoom(client, payload.RoomID)

	case "leave_room":
		var payload LeaveRoomPayload
		if err := json.Unmarshal(wsMsg.Payload, &payload); err != nil {
			log.Printf("Error unmarshalling leave_room payload: %v", err)
			return
		}
		client := h.findClientByUserID(wsMsg.ClientID)
		if client == nil {
			log.Printf("Error: client %s not found for leave_room", wsMsg.ClientID)
			return
		}
		h.handleLeaveRoom(client, payload.RoomID)

	default:
		log.Printf("Unknown message type received: %s", wsMsg.Type)
	}
}

func (h *Hub) handleJoinRoom(client *Client, roomID string) {
	h.mu.Lock() // Protects h.rooms
	room, ok := h.rooms[roomID]
	if !ok {
		// Room doesn't exist in hub, create it
		// In a real app, you might first check if roomID is a valid room via chatService.
		room = &RoomDetail{
			clients: make(map[*Client]bool),
			roomID:  roomID,
		}
		h.rooms[roomID] = room
		log.Printf("Room %s created in hub.", roomID)
	}
	h.mu.Unlock()

	room.mu.Lock()
	room.clients[client] = true
	room.mu.Unlock()

	client.joinRoom(roomID) // Update client's internal list of rooms

	log.Printf("Client %s (User: %s) joined room %s", client.conn.RemoteAddr(), client.userID, roomID)

	// Notify other clients in the room
	h.broadcastSystemMessage(roomID, client.userID, fmt.Sprintf("User %s joined the room.", client.userID), "user_joined")
}

func (h *Hub) handleLeaveRoom(client *Client, roomID string) {
	h.mu.RLock() // Use RLock as we are mostly reading h.rooms
	room, ok := h.rooms[roomID]
	h.mu.RUnlock()

	if !ok {
		log.Printf("Client %s tried to leave non-existent room %s in hub.", client.userID, roomID)
		return
	}

	room.mu.Lock()
	if _, stillInRoom := room.clients[client]; stillInRoom {
		delete(room.clients, client)
		log.Printf("Client %s (User: %s) left room %s", client.conn.RemoteAddr(), client.userID, roomID)
		if len(room.clients) == 0 {
			// Optional: remove room from hub if empty
			// h.mu.Lock()
			// delete(h.rooms, roomID)
			// h.mu.Unlock()
			// log.Printf("Room %s is now empty and removed from hub.", roomID)
		}
	}
	room.mu.Unlock()

	client.leaveRoom(roomID) // Update client's internal list of rooms

	// Notify other clients in the room
	h.broadcastSystemMessage(roomID, client.userID, fmt.Sprintf("User %s left the room.", client.userID), "user_left")
}


// broadcastToRoom sends a message to all clients in a specific room, optionally excluding one client.
func (h *Hub) broadcastToRoom(roomID string, wsMsg *WebsocketMessage, excludeClient *Client) {
	h.mu.RLock() // Read lock for accessing h.rooms
	room, ok := h.rooms[roomID]
	h.mu.RUnlock()

	if !ok {
		log.Printf("Cannot broadcast: Room %s not found in hub.", roomID)
		return
	}

	messageBytes, err := json.Marshal(wsMsg)
	if err != nil {
		log.Printf("Error marshalling message for broadcast to room %s: %v", roomID, err)
		return
	}

	room.mu.RLock() // Read lock for accessing room.clients
	defer room.mu.RUnlock()

	for client := range room.clients {
		if client == excludeClient {
			continue
		}
		select {
		case client.send <- messageBytes:
		default: // Failed to send, client's send buffer might be full
			log.Printf("Failed to send message to client %s in room %s (buffer full or closed). Unregistering.", client.userID, roomID)
			// It's risky to call unregister directly here due to potential deadlocks or recursive locking.
			// A common pattern is to schedule unregistration if send fails.
			// For simplicity here, we just log. In a robust system, you'd handle this more carefully.
			// close(client.send)
			// delete(h.clients, client) // This would need h.mu.Lock()
		}
	}
	log.Printf("Broadcast message of type '%s' to room %s", wsMsg.Type, roomID)
}

// broadcastSystemMessage is a helper to send system messages (like user joined/left).
func (h *Hub) broadcastSystemMessage(roomID, relevantUserID, content, messageType string) {
	payload := map[string]string{"user_id": relevantUserID, "content": content, "room_id": roomID}
	payloadBytes, _ := json.Marshal(payload)
	wsMsg := &WebsocketMessage{
		Type:    messageType,
		Payload: payloadBytes,
		RoomID:  roomID,
	}
	// Find the client associated with relevantUserID to exclude them if necessary,
	// though for "user_joined" or "user_left", they might not be in the broadcast list anyway
	// or it might be fine for them to receive it too.
	// For now, sending to all in room.
	h.broadcastToRoom(roomID, wsMsg, nil)
}


// findClientByUserID is a helper. This is inefficient.
// In a real system, you'd have a map[string]*Client for direct lookups if needed.
// For now, it's used by handleRouteMessage which gets ClientID (UserID) from the message.
func (h *Hub) findClientByUserID(userID string) *Client {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for client := range h.clients {
		if client.userID == userID {
			return client
		}
	}
	return nil
}
