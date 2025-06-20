package user

import "time"

// User represents a user in the chat application.
type User struct {
	ID           string    `json:"id" db:"id"`
	Username     string    `json:"username" db:"username"`
	PasswordHash string    `json:"-" db:"password_hash"` // '-' for json to omit
	AvatarURL    string    `json:"avatar_url,omitempty" db:"avatar_url"`
	Status       string    `json:"status,omitempty" db:"status"` // e.g., "online", "offline", "away"
	Description  string    `json:"description,omitempty" db:"description"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
}
