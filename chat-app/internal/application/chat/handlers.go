package chat_app

import (
	"context" // Will be needed
	"encoding/json"
	"errors" // Standard errors package
	"net/http"
	"strconv"
	// "strings" // No longer needed for path parameters with gorilla/mux

	"chat-app/internal/application/api_helpers"
	"chat-app/internal/domain/chat" // ChatService and Chat entities
	"chat-app/internal/infrastructure/auth" // For ContextKeys
	// "chat-app/internal/domain/user" // For user ID type consistency if needed
	"github.com/gorilla/mux" // For extracting path variables
)

// ChatHandler handles HTTP requests related to chat rooms and messages.
type ChatHandler struct {
	// Assuming chat.ChatServiceInterface is defined in domain/chat package
	chatService chat.ChatServiceInterface
}

// NewChatHandler creates a new ChatHandler.
func NewChatHandler(chatService chat.ChatServiceInterface) *ChatHandler {
	return &ChatHandler{chatService: chatService}
}

// CreateRoomRequest defines the expected JSON structure for creating a new room.
type CreateRoomRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	IsPrivate   bool   `json:"is_private,omitempty"`
}

// CreateRoom handles new chat room creation.
// POST /rooms
func (h *ChatHandler) CreateRoom(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		api_helpers.RespondWithError(w, http.StatusMethodNotAllowed, "Only POST method is allowed")
		return
	}

	creatorUserID, ok := r.Context().Value(auth.UserIDKey).(string)
	if !ok || creatorUserID == "" {
		api_helpers.RespondWithError(w, http.StatusUnauthorized, "User not authenticated or UserID missing from context")
		return
	}

	var req CreateRoomRequest
	err := api_helpers.DecodeJSONBody(r, &req)
	if err != nil {
		api_helpers.RespondWithError(w, http.StatusBadRequest, "Invalid request payload: "+err.Error())
		return
	}

	if req.Name == "" {
		api_helpers.RespondWithError(w, http.StatusBadRequest, "Room name is required")
		return
	}

	room, err := h.chatService.CreateRoom(r.Context(), req.Name, req.Description, creatorUserID, req.IsPrivate)
	if err != nil {
		if errors.Is(err, chat.ErrRoomNameTaken) { // Example of a specific domain error
			api_helpers.RespondWithError(w, http.StatusConflict, err.Error())
		} else {
			api_helpers.RespondWithError(w, http.StatusInternalServerError, "Failed to create room: "+err.Error())
		}
		return
	}
	api_helpers.RespondWithJSON(w, http.StatusCreated, room)
}

// ListPublicRooms handles listing public chat rooms with pagination.
// GET /rooms?limit=20&offset=0
func (h *ChatHandler) ListPublicRooms(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		api_helpers.RespondWithError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 {
		limit = 20 // Default limit
	}
	if offset < 0 {
		offset = 0 // Default offset
	}

	rooms, err := h.chatService.ListPublicRooms(r.Context(), limit, offset)
	if err != nil {
		api_helpers.RespondWithError(w, http.StatusInternalServerError, "Failed to list public rooms: "+err.Error())
		return
	}
	api_helpers.RespondWithJSON(w, http.StatusOK, rooms)
}

// ListUserRooms handles listing rooms for the authenticated user with pagination.
// GET /users/me/rooms?limit=20&offset=0 (or /users/{id}/rooms)
func (h *ChatHandler) ListUserRooms(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		api_helpers.RespondWithError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
		return
	}

	// This route is /users/me/rooms, so UserID comes from context.
	userID, ok := r.Context().Value(auth.UserIDKey).(string)
	if !ok || userID == "" {
		api_helpers.RespondWithError(w, http.StatusUnauthorized, "User not authenticated or UserID missing from context")
		return
	}
	// Example for path like /users/{userID}/rooms (if you had such a route)
	// pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	// if len(pathParts) >= 2 && pathParts[0] == "users" && pathParts[2] == "rooms" {
	// 	 userID = pathParts[1]
	// } else { ... error ... }


	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 {
		limit = 10
	}
	if offset < 0 {
		offset = 0
	}

	rooms, err := h.chatService.ListUserRooms(r.Context(), userID, limit, offset)
	if err != nil {
		api_helpers.RespondWithError(w, http.StatusInternalServerError, "Failed to list user rooms: "+err.Error())
		return
	}
	api_helpers.RespondWithJSON(w, http.StatusOK, rooms)
}

// GetRoomMessages handles fetching messages for a specific room with pagination.
// GET /rooms/{id}/messages?limit=50&offset=0
func (h *ChatHandler) GetRoomMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		api_helpers.RespondWithError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
		return
	}

	vars := mux.Vars(r)
	roomID, ok := vars["id"]
	if !ok || roomID == "" {
		api_helpers.RespondWithError(w, http.StatusBadRequest, "Room ID not found in path")
		return
	}

	// TODO: Authorization check: is user part of this room or is it a public room?
	// This could involve checking claims from context or making a service call.
	// For now, we assume if the user is authenticated, they can try to get messages.
	// The service layer (GetRoomMessages) might have further checks.
	_, authOk := r.Context().Value(auth.UserIDKey).(string)
	if !authOk {
		api_helpers.RespondWithError(w, http.StatusUnauthorized, "User not authenticated for fetching room messages")
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 {
		limit = 50 // Default limit
	}
	if offset < 0 {
		offset = 0 // Default offset
	}

	messages, err := h.chatService.GetRoomMessages(r.Context(), roomID, limit, offset)
	if err != nil {
		if errors.Is(err, chat.ErrRoomNotFound) { // Example
			api_helpers.RespondWithError(w, http.StatusNotFound, err.Error())
		} else {
			api_helpers.RespondWithError(w, http.StatusInternalServerError, "Failed to get room messages: "+err.Error())
		}
		return
	}
	api_helpers.RespondWithJSON(w, http.StatusOK, messages)
}


// Removed temporary ChatServiceInterface and custom errors struct.
// Assuming chat.ChatServiceInterface is correctly defined in the domain/chat package
// and standard "errors" package is used.
