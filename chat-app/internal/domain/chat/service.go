package chat

import (
	"context"
	"errors"
	"fmt"
	"time"

	"chat-app/internal/domain/user" // Importing the user domain package

	"github.com/google/uuid" // For generating IDs
)

var (
	ErrRoomNotFound         = errors.New("room not found")
	ErrMessageNotFound      = errors.New("message not found")
	ErrUserNotInRoom        = errors.New("user is not a member of the room")
	ErrUserAlreadyInRoom    = errors.New("user is already a member of the room")
	ErrPermissionDenied     = errors.New("permission denied")
	ErrRoomNameTaken        = errors.New("room name is already taken")
	ErrCannotLeaveRoom      = errors.New("cannot leave room (e.g., if user is the owner and no other owners exist)")
	ErrInvalidMessageType   = errors.New("invalid message type")
	ErrMessageContentEmpty  = errors.New("message content cannot be empty for text messages")
)

// ChatService provides business logic operations related to chat rooms and messages.
type ChatService struct {
	roomRepo    RoomRepository
	messageRepo MessageRepository
	// userRepo    user.UserRepository // Optional: if direct user checks beyond ID are needed
}

// NewChatService creates a new instance of ChatService.
func NewChatService(roomRepo RoomRepository, messageRepo MessageRepository) *ChatService {
	return &ChatService{
		roomRepo:    roomRepo,
		messageRepo: messageRepo,
	}
}

// CreateRoom creates a new chat room.
func (s *ChatService) CreateRoom(ctx context.Context, name string, description string, createdByUserID string, isPrivate bool) (*Room, error) {
	if name == "" {
		return nil, errors.New("room name cannot be empty")
	}

	// Optional: Check for room name uniqueness if required by business logic
	// existingRoom, err := s.roomRepo.FindByName(ctx, name)
	// if err != nil && !errors.Is(err, ErrRoomNotFound) { // Assuming ErrRoomNotFound from repo
	// 	return nil, fmt.Errorf("failed to check room name: %w", err)
	// }
	// if existingRoom != nil {
	// 	return nil, ErrRoomNameTaken
	// }

	newID, _ := uuid.NewRandom()
	room := &Room{
		ID:          newID.String(),
		Name:        name,
		Description: description,
		CreatedBy:   createdByUserID,
		IsPrivate:   isPrivate,
		Users:       make([]*user.User, 0), // Initialize as empty slice
		Messages:    make([]*Message, 0),   // Initialize as empty slice
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	if err := s.roomRepo.Create(ctx, room); err != nil {
		return nil, fmt.Errorf("failed to create room: %w", err)
	}

	// Automatically add creator to the room
	if err := s.roomRepo.AddUserToRoom(ctx, room.ID, createdByUserID); err != nil {
		// Log this error, but the room creation itself was successful.
		// This could be a more complex transaction in a real app.
		fmt.Printf("warning: failed to add creator %s to room %s: %v\n", createdByUserID, room.ID, err)
	}

	// Fetch the room again to get user details if AddUserToRoom populates them,
	// or handle user population within AddUserToRoom/FindByID if necessary.
	// For now, returning the room as created.
	return room, nil
}

// JoinRoom allows a user to join an existing public room or a private room if invited (invitation logic not implemented here).
func (s *ChatService) JoinRoom(ctx context.Context, roomID string, userID string) error {
	room, err := s.roomRepo.FindByID(ctx, roomID)
	if err != nil {
		if errors.Is(err, ErrRoomNotFound) {
			return ErrRoomNotFound
		}
		return fmt.Errorf("failed to find room: %w", err)
	}

	// For private rooms, invitation logic would be checked here.
	// For now, assuming any user can attempt to join if they know the ID.
	// The repository's AddUserToRoom should handle if user is already there.

	err = s.roomRepo.AddUserToRoom(ctx, roomID, userID)
	if err != nil {
		// The repository should ideally return a specific error if user is already in room.
		// For now, we assume a generic error could mean that or other issues.
		return fmt.Errorf("failed to add user to room: %w", err)
	}
	// Could publish an event "user joined room"
	return nil
}

// LeaveRoom allows a user to leave a room.
func (s *ChatService) LeaveRoom(ctx context.Context, roomID string, userID string) error {
	room, err := s.roomRepo.FindByID(ctx, roomID)
	if err != nil {
		return ErrRoomNotFound
	}

	// Business rule: e.g., owner cannot leave if they are the only one, or ownership must be transferred.
	// This logic can be complex and is simplified here.
	if room.CreatedBy == userID {
		// Placeholder: Check if other owners/admins exist or if room should be deleted.
		// For now, let's assume owner can leave.
	}

	err = s.roomRepo.RemoveUserFromRoom(ctx, roomID, userID)
	if err != nil {
		// The repository should ideally return a specific error if user is not in room.
		return fmt.Errorf("failed to remove user from room: %w", err)
	}
	// Could publish an event "user left room"
	return nil
}

// SendMessage creates a new message in a room.
func (s *ChatService) SendMessage(ctx context.Context, roomID string, userID string, content string, messageType MessageType) (*Message, error) {
	if messageType == TextMessageType && content == "" {
		return nil, ErrMessageContentEmpty
	}
	// Validate messageType
	switch messageType {
	case TextMessageType, FileMessageType, ImageMessageType, EmojiMessageType, SystemMessageType:
		// valid
	default:
		return nil, ErrInvalidMessageType
	}

	// Check if user is part of the room (optional, could be handled by UI flow or deeper checks)
	// For simplicity, assuming if a user can send, they are authorized.
	// More robust check: use roomRepo.IsUserInRoom(ctx, roomID, userID)

	newID, _ := uuid.NewRandom()
	message := &Message{
		ID:        newID.String(),
		RoomID:    roomID,
		UserID:    userID, // For system messages, UserID might be empty or a special system ID
		Content:   content,
		Timestamp: time.Now().UTC(),
		Type:      messageType,
		Status:    MessageSent, // Initial status
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	if err := s.messageRepo.Create(ctx, message); err != nil {
		return nil, fmt.Errorf("failed to send message: %w", err)
	}
	// Could publish an event "new message in room" to a websocket hub
	return message, nil
}

// GetRoomMessages retrieves messages for a specific room with pagination.
func (s *ChatService) GetRoomMessages(ctx context.Context, roomID string, limit, offset int) ([]*Message, error) {
	// Validate limit and offset
	if limit <= 0 {
		limit = 20 // Default limit
	}
	if offset < 0 {
		offset = 0 // Default offset
	}

	messages, err := s.messageRepo.FindByRoomID(ctx, roomID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get room messages: %w", err)
	}
	return messages, nil
}

// GetRoomInfo retrieves details of a specific room.
func (s *ChatService) GetRoomInfo(ctx context.Context, roomID string) (*Room, error) {
	room, err := s.roomRepo.FindByID(ctx, roomID)
	if err != nil {
		if errors.Is(err, ErrRoomNotFound) {
			return nil, ErrRoomNotFound
		}
		return nil, fmt.Errorf("failed to get room info: %w", err)
	}
	// The Room object from repo might contain list of User objects or user IDs.
	// If it contains user IDs, service might need to fetch user details if required by API contract.
	return room, nil
}

// ListUserRooms lists all rooms a specific user is part of.
func (s *ChatService) ListUserRooms(ctx context.Context, userID string, limit, offset int) ([]*Room, error) {
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	rooms, err := s.roomRepo.ListRoomsForUser(ctx, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list user rooms: %w", err)
	}
	return rooms, nil
}

// ListPublicRooms lists all public rooms with pagination.
func (s *ChatService) ListPublicRooms(ctx context.Context, limit, offset int) ([]*Room, error) {
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	rooms, err := s.roomRepo.ListPublicRooms(ctx, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list public rooms: %w", err)
	}
	return rooms, nil
}
