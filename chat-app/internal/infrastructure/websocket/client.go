package websocket

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second
	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second
	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10
	// Maximum message size allowed from peer.
	maxMessageSize = 1024 * 4 // 4KB
)

var (
	newline = []byte{'\n'}
	space   = []byte{' '}
)

// Client is a middleman between the websocket connection and the hub.
type Client struct {
	hub *Hub
	// The websocket connection.
	conn *websocket.Conn
	// Buffered channel of outbound messages.
	send   chan []byte
	userID string // Authenticated user ID
	// Rooms this client is currently subscribed to. Key is roomID.
	rooms map[string]bool
	mu    sync.Mutex // Protects rooms map
}

// NewClient creates a new Client.
func NewClient(hub *Hub, conn *websocket.Conn, userID string) *Client {
	return &Client{
		hub:    hub,
		conn:   conn,
		send:   make(chan []byte, 256), // Buffered channel
		userID: userID,
		rooms:  make(map[string]bool),
	}
}

// readPump pumps messages from the websocket connection to the hub.
//
// The application runs readPump in a per-connection goroutine. The application
// ensures that there is at most one reader on a connection by executing all
// reads from this goroutine.
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()
	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error { c.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error: %v", err)
			}
			break // Connection closed or error
		}
		message = bytes.TrimSpace(bytes.Replace(message, newline, space, -1))

		// For now, assume message is a JSON that can be decoded into WebsocketMessage
		// The Hub will process this message.
		// In a more complex system, client might handle some messages itself (e.g. pings)
		// or parse into a more structured command for the hub.

		// Create a generic WebsocketMessage to send to the hub's broadcast channel
		// The actual processing and routing logic will be in the hub.
		// Here, we wrap the raw message with sender's info.
		// This is a simplified approach. A better one would be to unmarshal into a specific struct
		// that dictates action (e.g., join room, send message).

		// Let's assume the client sends JSON messages like:
		// {"type": "send_message", "payload": {"room_id": "xyz", "content": "hello"}}
		// {"type": "join_room", "payload": {"room_id": "abc"}}
		// {"type": "leave_room", "payload": {"room_id": "abc"}}

		var wsMsg WebsocketMessage
		if err := json.Unmarshal(message, &wsMsg); err != nil {
			log.Printf("Error unmarshalling websocket message from client %s: %v. Message: %s", c.userID, err, string(message))
			// Optionally send an error message back to client
			// c.send <- []byte(`{"error": "invalid message format"}`)
			continue
		}
		wsMsg.ClientID = c.userID // Ensure ClientID is set from authenticated user

		// Instead of directly broadcasting, send to a Hub channel for processing.
		// This allows the Hub to decide what to do (e.g. persist, then broadcast, or handle room join).
		c.hub.routeMessage <- &wsMsg // Send structured message to hub for routing/processing
	}
}

// writePump pumps messages from the hub to the websocket connection.
//
// A goroutine running writePump is started for each connection. The
// application ensures that there is at most one writer to a connection by
// executing all writes from this goroutine.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// The hub closed the channel.
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Add queued chat messages to the current websocket message.
			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write(newline)
				w.Write(<-c.send)
			}

			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// Helper methods for client to manage its rooms
func (c *Client) joinRoom(roomID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rooms[roomID] = true
}

func (c *Client) leaveRoom(roomID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.rooms, roomID)
}

func (c *Client) isInRoom(roomID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.rooms[roomID]
	return ok
}
