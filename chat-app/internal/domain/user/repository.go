package user

import "context"

// UserRepository defines the interface for interacting with user storage.
type UserRepository interface {
	Create(ctx context.Context, user *User) error
	FindByID(ctx context.Context, id string) (*User, error)
	FindByUsername(ctx context.Context, username string) (*User, error)
	Update(ctx context.Context, user *User) error
	Delete(ctx context.Context, id string) error
	// Add other methods like ListAll, FindByEmail if necessary
}
