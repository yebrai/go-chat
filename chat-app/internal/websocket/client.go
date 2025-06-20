package websocket

import (
	"encoding/json"
	"log"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// writeWait is the time allowed to write a message to the peer.
	writeWait = 10 * time.Second
	// pongWait is the time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second
	// pingPeriod is the period for sending pings to peer. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10
	// maxMessageSize is the maximum message size allowed from a peer.
	maxMessageSize = 2048 // 2KB, can be adjusted based on expected message sizes.
)

// Client represents a connected WebSocket client. It acts as a bridge
// between the WebSocket connection and the central Hub. Each client runs
// in its own goroutines for reading and writing messages.
type Client struct {
	hub           *Hub            // Reference to the central Hub.
	conn          *websocket.Conn // The underlying WebSocket connection.
	send          chan *Message   // Buffered channel for outbound messages to this client.
	username      string          // Username of the connected user.
	currentRoomID string          // The ID of the room the client is currently active in.
}

// NewClient creates and returns a new Client instance.
// It requires the Hub, the WebSocket connection, the client's username,
// and the initial roomID the client intends to join.
func NewClient(hub *Hub, conn *websocket.Conn, username string, initialRoomID string) *Client {
	return &Client{
		hub:           hub,
		conn:          conn,
		send:          make(chan *Message, 256), // Buffered channel for outbound messages.
		username:      username,
		currentRoomID: initialRoomID, // Set upon connection, Hub handles actual join.
	}
}

// readPump pumps messages from the WebSocket connection to the Hub.
// This method runs in a dedicated goroutine for each client. It ensures that
// there is at most one reader on a connection by executing all reads from this goroutine.
// Messages read are unmarshaled into the `Message` struct and forwarded to the Hub's
// `routeMessage` channel for processing.
func (c *Client) ReadPump() {
	defer func() {
		// When readPump exits (due to error or connection close), unregister the client
		// and close the WebSocket connection.
		c.hub.unregister <- c
		c.conn.Close()
		log.Printf("CLIENT: User '%s' (room '%s') disconnected. readPump stopped.", c.username, c.currentRoomID)
	}()

	c.conn.SetReadLimit(maxMessageSize)
	// Set initial read deadline. This is refreshed by the pong handler.
	if err := c.conn.SetReadDeadline(time.Now().Add(pongWait)); err != nil {
		log.Printf("CLIENT: Error setting read deadline for %s: %v", c.username, err)
		// Not returning here, as the connection might still be usable or close gracefully.
	}
	c.conn.SetPongHandler(func(string) error {
		// When a pong is received, extend the read deadline.
		if err := c.conn.SetReadDeadline(time.Now().Add(pongWait)); err != nil {
			log.Printf("CLIENT: Error setting read deadline on pong for %s: %v", c.username, err)
			// Depending on strictness, might want to terminate connection here.
		}
		return nil
	})

	for {
		// Read a message from the WebSocket connection.
		// conn.ReadMessage() returns message type, payload, and error.
		_, rawMessage, err := c.conn.ReadMessage()
		if err != nil {
			// Log different types of errors. IsUnexpectedCloseError helps distinguish
			// between normal closures (e.g., browser tab closed) and actual network issues.
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure, websocket.CloseNormalClosure) {
				log.Printf("CLIENT: User '%s' read error: %v", c.username, err)
			} else {
				// This could be a normal closure (e.g. CloseNormalClosure if client explicitly closes)
				log.Printf("CLIENT: User '%s' connection closed (potentially normally): %v", c.username, err)
			}
			break // Exit the loop, which triggers unregistration via defer.
		}

		// Unmarshal the raw message into our standard Message struct.
		var msg Message
		if err := json.Unmarshal(rawMessage, &msg); err != nil {
			log.Printf("CLIENT: User '%s' sent invalid JSON: %v. Raw message: %s", c.username, err, string(rawMessage))
			// Optionally, send an error message back to the client.
			// This requires careful handling to avoid blocking if the client's send channel is full.
			// Example: errorMsg := &Message{Type: ErrorMessageType, Content: "Invalid message format", Timestamp: time.Now()}
			// select { case c.send <- errorMsg: default: log.Printf("CLIENT: Failed to send error to %s, send channel full", c.username)}
			continue // Skip processing this malformed message.
		}

		// Populate message with server-authoritative information.
		msg.Username = c.username        // Sender's username from the authenticated client session.
		msg.Timestamp = time.Now().UTC() // Server-side timestamp for received message before routing.

		// If the message type implies it's for the client's current room and RoomID is missing,
		// set it. For messages like JoinRoom, RoomID is in payload/data.
		if msg.RoomID == "" {
			switch msg.Type {
			case TextMessageType, UserTypingMessageType, RequestStatsType: // Types that implicitly target current room
				msg.RoomID = c.currentRoomID
			}
		}
		// Note: The Hub will ultimately decide which room a message is routed to or affects,
		// especially for JoinRoomMessageType where msg.Data contains the target room.

		// Send the structured message to the Hub for central processing.
		select {
		case c.hub.routeMessage <- &msg:
		default:
			// Hub's routeMessage channel is full. This indicates a bottleneck in the Hub.
			// Log this issue. Depending on design, might disconnect client or drop message.
			log.Printf("CLIENT: Hub routeMessage channel full for user '%s'. Message of type '%s' dropped.", c.username, msg.Type)
			// To prevent client from being stuck if hub is overloaded, we might close connection here.
			// For now, just dropping the message.
		}
	}
}

// writePump pumps messages from the Hub (via the client's send channel) to the WebSocket connection.
// This method runs in a dedicated goroutine for each client. It ensures that
// there is at most one writer on a connection by executing all writes from this goroutine.
func (c *Client) WritePump() {
	// Start a ticker to send ping messages periodically.
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		// When writePump exits, ensure the WebSocket connection is closed.
		// Unregistration is handled by readPump's exit or Hub logic.
		c.conn.Close()
		log.Printf("CLIENT: User '%s' writePump stopped.", c.username)
	}()

	for {
		select {
		case message, ok := <-c.send:
			// Set a deadline for writing the message to the peer.
			if err := c.conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
				log.Printf("CLIENT: Error setting write deadline for %s: %v", c.username, err)
				return // Assume connection is broken.
			}
			if !ok {
				// The Hub closed the client's send channel. This signifies that the client
				// should be disconnected. Send a WebSocket close message.
				log.Printf("CLIENT: Hub closed send channel for user '%s'. Sending close message.", c.username)
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			// Write the message to the WebSocket connection as JSON.
			if err := c.conn.WriteJSON(message); err != nil {
				log.Printf("CLIENT: Error writing JSON to user '%s': %v", c.username, err)
				// Assume connection is broken. readPump will likely catch this and unregister.
				return
			}
			log.Printf("CLIENT: Sent message type '%s' to user '%s' in room '%s'", message.Type, c.username, message.RoomID)

		case <-ticker.C:
			// Send a ping message to the peer.
			if err := c.conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
				log.Printf("CLIENT: Error setting write deadline for ping to %s: %v", c.username, err)
				return
			}
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Printf("CLIENT: Error sending ping to user '%s': %v", c.username, err)
				return // Assume connection is broken.
			}
		}
	}
}
