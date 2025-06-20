package websocket

import "time"

// MessageType is a string type representing the various types of messages
// that can be sent over a WebSocket connection in the chat application.
type MessageType string

// Constants for different WebSocket message types.
// These define the protocol for client-server communication regarding chat events.
const (
	// TextMessageType is for standard chat text messages from a user.
	// Direction: Client to Server (C2S) and Server to Client (S2C).
	TextMessageType MessageType = "text_message"

	// UserJoinedMessageType indicates a user has joined a room.
	// Direction: Server to Client (S2C).
	UserJoinedMessageType MessageType = "user_joined"

	// UserLeftMessageType indicates a user has left a room.
	// Direction: Server to Client (S2C).
	UserLeftMessageType MessageType = "user_left"

	// RoomStatsUpdateType provides an update on room statistics (e.g., user count, message count).
	// Direction: Server to Client (S2C).
	RoomStatsUpdateType MessageType = "room_stats_update" // Changed from "room_stats" to be more descriptive for S2C push

	// RecentMessagesType provides a list of recent messages, typically sent when a user joins a room.
	// Direction: Server to Client (S2C).
	RecentMessagesType MessageType = "recent_messages"

	// UserListUpdateType provides an updated list of users currently in a room.
	// Direction: Server to Client (S2C).
	UserListUpdateType MessageType = "user_list_update"

	// GlobalUserCountUpdateType provides an update on the total number of globally connected users.
	// Direction: Server to Client (S2C).
	GlobalUserCountUpdateType MessageType = "global_user_count_update"

	// ErrorMessageType is used by the server to send an error message to a specific client.
	// Direction: Server to Client (S2C).
	ErrorMessageType MessageType = "error_message"

	// JoinRoomMessageType is sent by a client when they want to join or switch to a specific room.
	// Direction: Client to Server (C2S).
	JoinRoomMessageType MessageType = "join_room"

	// LeaveRoomMessageType is sent by a client to explicitly leave their current room (without disconnecting).
	// Direction: Client to Server (C2S).
	LeaveRoomMessageType MessageType = "leave_room"

	// UserTypingMessageType indicates a user is currently typing a message.
	// The `content` field can be "start" or "stop".
	// Direction: Client to Server (C2S) and Server to Client (S2C).
	UserTypingMessageType MessageType = "user_typing"

	// RequestStatsType is sent by a client to request current statistics for a specific room.
	// Direction: Client to Server (C2S).
	RequestStatsType MessageType = "request_room_stats" // Changed from "request_stats" for clarity
)

// Message is the primary structure for messages exchanged over WebSocket.
// It defines a common format for various types of information, including
// chat texts, system notifications, and data payloads.
type Message struct {
	// Type indicates the kind of message, determining how payload/data should be interpreted.
	Type MessageType `json:"type"`
	// Content is primarily used for text messages or simple string payloads (e.g., system notifications).
	Content string `json:"content,omitempty"`
	// Username identifies the sender of the message, or the user relevant to a system event.
	Username string `json:"username,omitempty"`
	// RoomID specifies the room this message pertains to. For global messages, this might be empty.
	RoomID string `json:"roomID,omitempty"`
	// Timestamp records when the message was generated, typically set by the server for S2C messages.
	Timestamp time.Time `json:"timestamp"`
	// System is a boolean flag indicating if this is a system-generated message (e.g., join/leave notifications)
	// rather than a user-generated chat message.
	System bool `json:"system,omitempty"`
	// Data is a flexible field for more complex payloads, such as lists of messages, user lists, or structured stats.
	// The actual type of Data depends on the MessageType.
	Data interface{} `json:"data,omitempty"`
}

// --- Specific Payload Structures for Message.Data ---

// RoomStatsPayload defines the structured data for RoomStatsUpdateType messages.
// It contains various statistics related to a specific chat room.
type RoomStatsPayload struct {
	RoomID       string `json:"roomID"`        // The ID of the room these stats pertain to.
	ActiveUsers  int64  `json:"active_users"`  // The current number of active users in the room.
	MessageCount int64  `json:"message_count"` // The total number of messages ever sent in the room.
}

// UserListPayload defines the structured data for UserListUpdateType messages.
// It provides a list of usernames currently active in a specific room.
type UserListPayload struct {
	RoomID string   `json:"roomID"` // The ID of the room this user list is for.
	Users  []string `json:"users"`  // A slice of usernames.
}

// RecentMessagesPayload defines the structured data for RecentMessagesType messages.
// It contains a list of recent messages, where each message is a JSON string
// (typically a serialized domain.Message or another websocket.Message of TextMessageType).
type RecentMessagesPayload struct {
	RoomID   string   `json:"roomID"`   // The room these messages belong to.
	Messages []string `json:"messages"` // Slice of serialized message objects.
}

// GlobalUserCountPayload defines the structured data for GlobalUserCountUpdateType messages.
// It provides the total count of currently connected users across all rooms.
type GlobalUserCountPayload struct {
	Count int64 `json:"count"` // The total number of globally active users.
}

// ErrorPayload defines structured data for ErrorMessageType messages.
// It allows sending a more detailed error back to the client.
type ErrorPayload struct {
	Code    int    `json:"code,omitempty"` // An optional application-specific error code.
	Message string `json:"message"`        // A descriptive error message.
}

// JoinRoomData is the expected structure within Message.Content or Message.Data
// when a client sends a JoinRoomMessageType.
type JoinRoomData struct {
	RoomID string `json:"roomID"` // The ID of the room the client wishes to join.
}

// TypingData is the expected structure for UserTypingMessageType, typically in Message.Data.
// Alternatively, Message.Content could be "start" or "stop".
// Using a struct in Data allows for more extensibility if needed.
type TypingData struct {
	Username string `json:"username"`          // The username of the user who is typing.
	RoomID   string `json:"roomID"`            // The room where typing is occurring.
	Status   string `json:"status"`            // "start" or "stop"
}

// PlaceholderMessage function, can be removed if not needed.
// It was part of the initial scaffolding.
func PlaceholderMessage() string {
	return "WebSocket message definitions are structured here."
}
