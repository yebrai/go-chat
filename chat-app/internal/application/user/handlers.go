package user_app

import (
	"context" // Will be needed eventually for service calls
	"encoding/json"
	"errors" // Standard errors package
	"net/http"
	// "strings" // No longer needed for path parameters with gorilla/mux

	"chat-app/internal/application/api_helpers"
	"chat-app/internal/domain/user" // UserService and User entity
	"chat-app/internal/infrastructure/auth" // For JWTService and ContextKeys
	"github.com/gorilla/mux"      // For extracting path variables
)

// UserHandler handles HTTP requests related to users.
type UserHandler struct {
	userService user.UserServiceInterface // Assuming user.UserServiceInterface from domain
	jwtService  auth.TokenGenerator       // Interface for JWT generation/validation
	// Or use concrete type: jwtService *auth.JWTService
}

// TokenGenerator interface for JWTService (can be defined in auth package or here)
// This allows for easier testing and adheres to dependency inversion.
// type TokenGenerator interface {
// GenerateToken(userID string, username string) (string, error)
// GenerateRefreshToken(userID string) (string, error)
// ValidateToken(tokenString string) (*auth.CustomClaims, error)
// }

// NewUserHandler creates a new UserHandler.
func NewUserHandler(userService user.UserServiceInterface, jwtService auth.TokenGenerator) *UserHandler {
	return &UserHandler{
		userService: userService,
		jwtService:  jwtService,
	}
}

// RegistrationRequest defines the expected JSON structure for user registration.
type RegistrationRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Register handles new user registration.
// POST /users/register
func (h *UserHandler) Register(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		api_helpers.RespondWithError(w, http.StatusMethodNotAllowed, "Only POST method is allowed")
		return
	}

	var req RegistrationRequest
	err := api_helpers.DecodeJSONBody(r, &req)
	if err != nil {
		api_helpers.RespondWithError(w, http.StatusBadRequest, "Invalid request payload: "+err.Error())
		return
	}

	if req.Username == "" || req.Password == "" {
		api_helpers.RespondWithError(w, http.StatusBadRequest, "Username and password are required")
		return
	}

	// TODO: Add more robust validation (e.g., password complexity, username format)

	createdUser, err := h.userService.RegisterUser(r.Context(), req.Username, req.Password)
	if err != nil {
		// Determine appropriate status code based on error type
		if errors.Is(err, user.ErrUsernameTaken) || errors.Is(err, user.ErrPasswordTooShort) {
			api_helpers.RespondWithError(w, http.StatusConflict, err.Error())
		} else {
			api_helpers.RespondWithError(w, http.StatusInternalServerError, "Failed to register user: "+err.Error())
		}
		return
	}

	// Omit password hash from response
	createdUser.PasswordHash = ""
	api_helpers.RespondWithJSON(w, http.StatusCreated, createdUser)
}

// LoginRequest defines the expected JSON structure for user login.
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Login handles user authentication.
// POST /users/login
func (h *UserHandler) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		api_helpers.RespondWithError(w, http.StatusMethodNotAllowed, "Only POST method is allowed")
		return
	}
	var req LoginRequest
	err := api_helpers.DecodeJSONBody(r, &req)
	if err != nil {
		api_helpers.RespondWithError(w, http.StatusBadRequest, "Invalid request payload: "+err.Error())
		return
	}

	authenticatedUser, err := h.userService.AuthenticateUser(r.Context(), req.Username, req.Password)
	if err != nil {
		if errors.Is(err, user.ErrInvalidCredentials) {
			api_helpers.RespondWithError(w, http.StatusUnauthorized, err.Error())
		} else {
			api_helpers.RespondWithError(w, http.StatusInternalServerError, "Login failed: "+err.Error())
		}
		return
	}

	accessToken, err := h.jwtService.GenerateToken(authenticatedUser.ID, authenticatedUser.Username)
	if err != nil {
		api_helpers.RespondWithError(w, http.StatusInternalServerError, "Failed to generate access token: "+err.Error())
		return
	}

	refreshToken, err := h.jwtService.GenerateRefreshToken(authenticatedUser.ID)
	if err != nil {
		api_helpers.RespondWithError(w, http.StatusInternalServerError, "Failed to generate refresh token: "+err.Error())
		return
	}

	authenticatedUser.PasswordHash = "" // Omit password hash from response
	api_helpers.RespondWithJSON(w, http.StatusOK, map[string]interface{}{
		"message":       "Login successful",
		"user":          authenticatedUser,
		"access_token":  accessToken,
		"refresh_token": refreshToken,
	})
}

// RefreshTokenRequest defines the expected JSON structure for token refresh.
type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// RefreshToken handles generating a new access token from a refresh token.
// POST /auth/refresh
func (h *UserHandler) RefreshToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		api_helpers.RespondWithError(w, http.StatusMethodNotAllowed, "Only POST method is allowed")
		return
	}

	var req RefreshTokenRequest
	if err := api_helpers.DecodeJSONBody(r, &req); err != nil {
		api_helpers.RespondWithError(w, http.StatusBadRequest, "Invalid request payload: "+err.Error())
		return
	}

	if req.RefreshToken == "" {
		api_helpers.RespondWithError(w, http.StatusBadRequest, "Refresh token is required")
		return
	}

	claims, err := h.jwtService.ValidateToken(req.RefreshToken)
	if err != nil {
		api_helpers.RespondWithError(w, http.StatusUnauthorized, "Invalid or expired refresh token: "+err.Error())
		return
	}

	if claims.Type != auth.TokenTypeRefresh {
		api_helpers.RespondWithError(w, http.StatusUnauthorized, "Invalid token type: not a refresh token")
		return
	}

	// Get user details (username) to generate a new access token
	// Username might not be in refresh token claims, so fetch user if needed.
	// For this JWTService, username is optional in refresh token.
	// If not present, we might need to fetch the user by claims.UserID.
	// For simplicity, let's assume claims.Username is available or not strictly needed by GenerateToken
	// if GenerateToken can work with just UserID for access tokens (it currently expects username).
	// Our current GenerateToken expects username. If refresh token doesn't have it, we must fetch user.

	var usernameForNewToken = claims.Username
	if usernameForNewToken == "" {
		// Refresh token does not contain username, fetch user from DB
		userFromDB, errUser := h.userService.GetUserByID(r.Context(), claims.UserID)
		if errUser != nil {
			api_helpers.RespondWithError(w, http.StatusUnauthorized, "User associated with refresh token not found")
			return
		}
		usernameForNewToken = userFromDB.Username
	}


	newAccessToken, err := h.jwtService.GenerateToken(claims.UserID, usernameForNewToken)
	if err != nil {
		api_helpers.RespondWithError(w, http.StatusInternalServerError, "Failed to generate new access token: "+err.Error())
		return
	}

	// Optionally, issue a new refresh token (for rotation)
	// newRefreshToken, err := h.jwtService.GenerateRefreshToken(claims.UserID)
	// if err != nil { ... }

	api_helpers.RespondWithJSON(w, http.StatusOK, map[string]string{
		"access_token": newAccessToken,
		// "refresh_token": newRefreshToken, // If rotating
	})
}


// GetUserProfile handles fetching a user's profile.
// GET /users/{id}
func (h *UserHandler) GetUserProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		api_helpers.RespondWithError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
		return
	}

	vars := mux.Vars(r)
	userID, ok := vars["id"]
	if !ok || userID == "" {
		api_helpers.RespondWithError(w, http.StatusBadRequest, "User ID not found in path")
		return
	}

	// TODO: Implement authorization:
	// - If {id} is "me" or matches authenticated user ID, allow.
	// - Admins might be able to fetch any profile.
	// For now, allow fetching any profile by ID.

	profileUser, err := h.userService.GetUserByID(r.Context(), userID)
	if err != nil {
		if errors.Is(err, user.ErrUserNotFound) {
			api_helpers.RespondWithError(w, http.StatusNotFound, err.Error())
		} else {
			api_helpers.RespondWithError(w, http.StatusInternalServerError, "Failed to get user profile: "+err.Error())
		}
		return
	}

	profileUser.PasswordHash = "" // Never expose password hash
	api_helpers.RespondWithJSON(w, http.StatusOK, profileUser)
}

// UpdateUserProfileRequest defines the structure for updating a user's profile.
type UpdateUserProfileRequest struct {
	Username    string `json:"username,omitempty"`
	AvatarURL   string `json:"avatar_url,omitempty"`
	Description string `json:"description,omitempty"`
	// Password changes should be handled via a separate endpoint/flow.
}

// UpdateUserProfile handles updates to the authenticated user's profile.
// PUT /users/profile  (Implicitly for the authenticated user)
func (h *UserHandler) UpdateUserProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		api_helpers.RespondWithError(w, http.StatusMethodNotAllowed, "Only PUT method is allowed")
		return
	}

	// Get authenticated UserID from context
	authUserID, ok := r.Context().Value(auth.UserIDKey).(string)
	if !ok || authUserID == "" {
		api_helpers.RespondWithError(w, http.StatusUnauthorized, "User not authenticated or UserID missing from context")
		return
	}

	// Users should only be able to update their own profile.
	// If you wanted admins to update any profile, you'd need role checks here
	// and the target userID might come from a path variable.
	// For /users/profile, it's implicitly the authenticated user.

	var req UpdateUserProfileRequest
	err := api_helpers.DecodeJSONBody(r, &req)
	if err != nil {
		api_helpers.RespondWithError(w, http.StatusBadRequest, "Invalid request payload: "+err.Error())
		return
	}

	// Basic validation: at least one field must be present for update, or allow empty request to mean "no change"?
	// For now, service layer handles if fields are empty.

	updatedUser, err := h.userService.UpdateUserProfile(r.Context(), authUserID, req.Username, req.AvatarURL, req.Description)
	if err != nil {
		if errors.Is(err, user.ErrUserNotFound) {
			api_helpers.RespondWithError(w, http.StatusNotFound, err.Error())
		} else if errors.Is(err, user.ErrUsernameTaken) {
			api_helpers.RespondWithError(w, http.StatusConflict, err.Error())
		} else {
			api_helpers.RespondWithError(w, http.StatusInternalServerError, "Failed to update user profile: "+err.Error())
		}
		return
	}

	updatedUser.PasswordHash = ""
	api_helpers.RespondWithJSON(w, http.StatusOK, updatedUser)
}

// Ensure auth.TokenGenerator interface matches methods in auth.JWTService
// type TokenGenerator interface {
//    GenerateToken(userID string, username string) (string, error)
//    GenerateRefreshToken(userID string) (string, error)
//    ValidateToken(tokenString string) (*CustomClaims, error)
//}
// This should be defined in the auth package or a shared types package.
// For now, assuming JWTService directly implements these methods and can be used.
// If JWTService is used directly, change type in UserHandler struct and NewUserHandler.
// Let's assume JWTService is used directly for simplicity now.

// Define user.UserServiceInterface if not already defined in domain/user.
// Example:
// type UserServiceInterface interface {
//     RegisterUser(ctx context.Context, username string, password string) (*user.User, error)
//     AuthenticateUser(ctx context.Context, username string, password string) (*user.User, error)
//     GetUserByID(ctx context.Context, id string) (*user.User, error)
//     UpdateUserProfile(ctx context.Context, userID string, newUsername, newAvatarURL, newDescription string) (*user.User, error)
// }
// This should be in domain/user/service.go (or an interfaces file there)
