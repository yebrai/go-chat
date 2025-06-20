package chat

import "context"

// RoomRepository defines the interface for interacting with room storage.
type RoomRepository interface {
	Create(ctx context.Context, room *Room) error
	FindByID(ctx context.Context, id string) (*Room, error)
	FindByName(ctx context.Context, name string) (*Room, error) // Added for completeness
	AddUserToRoom(ctx context.Context, roomID string, userID string) error
	RemoveUserFromRoom(ctx context.Context, roomID string, userID string) error
	Update(ctx context.Context, room *Room) error
	Delete(ctx context.Context, id string) error
	ListPublicRooms(ctx context.Context, limit, offset int) ([]*Room, error) // Added for listing rooms
	ListRoomsForUser(ctx context.Context, userID string, limit, offset int) ([]*Room, error) // Added for user-specific rooms
}

// MessageRepository defines the interface for interacting with message storage.
type MessageRepository interface {
	Create(ctx context.Context, message *Message) error
	FindByID(ctx context.Context, id string) (*Message, error)
	FindByRoomID(ctx context.Context, roomID string, limit, offset int) ([]*Message, error)
	Update(ctx context.Context, message *Message) error // e.g., for message status
	Delete(ctx context.Context, id string) error        // Soft or hard delete
	// Add other methods like FindByUser, SearchMessages if necessary
}
