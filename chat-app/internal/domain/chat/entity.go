package chat

import (
	"time"

	// Assuming 'chat-app' is the root of the Go module.
	// Adjust the import path if the module name is different.
	// For example, if your go.mod defines module "mychat",
	// then it would be "mychat/internal/domain/user".
	// For now, using a placeholder common for local development.
	"chat-app/internal/domain/user"
)

// MessageType defines the type of a message.
type MessageType string

const (
	TextMessageType  MessageType = "text"
	FileMessageType  MessageType = "file"
	ImageMessageType MessageType = "image"
	EmojiMessageType MessageType = "emoji"
	SystemMessageType MessageType = "system" // For system messages like "User joined"
)

// MessageStatus defines the status of a message.
type MessageStatus string

const (
	MessageSent      MessageStatus = "sent"
	MessageDelivered MessageStatus = "delivered"
	MessageRead      MessageStatus = "read"
	MessageFailed    MessageStatus = "failed"
)

// Message represents a chat message within a room.
type Message struct {
	ID        string        `json:"id" db:"id"`
	RoomID    string        `json:"room_id" db:"room_id"`
	UserID    string        `json:"user_id" db:"user_id"` // Can be empty for system messages
	User      *user.User    `json:"user,omitempty" db:"-"` // Embedded user info, not stored in messages table directly
	Content   string        `json:"content" db:"content"`
	Timestamp time.Time     `json:"timestamp" db:"timestamp"`
	Type      MessageType   `json:"type" db:"type"`
	Status    MessageStatus `json:"status,omitempty" db:"status"`
	Metadata  map[string]interface{} `json:"metadata,omitempty" db:"metadata"` // For file URLs, image dimensions, etc.
	CreatedAt time.Time     `json:"created_at" db:"created_at"`
	UpdatedAt time.Time     `json:"updated_at" db:"updated_at"`
}

// Room represents a chat room where users can exchange messages.
type Room struct {
	ID          string     `json:"id" db:"id"`
	Name        string     `json:"name" db:"name"`
	Description string     `json:"description,omitempty" db:"description"`
	CreatedBy   string     `json:"created_by" db:"created_by"` // UserID of the creator
	Users       []*user.User `json:"users,omitempty" db:"-"`     // List of users in the room, likely managed via a join table
	Messages    []*Message `json:"messages,omitempty" db:"-"`  // Recent messages, usually paginated and not stored directly in room document
	IsPrivate   bool       `json:"is_private,omitempty" db:"is_private"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" db:"updated_at"`
}
