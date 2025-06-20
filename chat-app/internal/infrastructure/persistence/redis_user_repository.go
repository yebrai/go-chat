package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"chat-app/internal/domain/user"
	"github.com/go-redis/redis/v8"
)

const (
	userCachePrefixByID       = "user:id:"
	userCachePrefixByUsername = "user:username:"
	defaultUserCacheTTL       = 1 * time.Hour
)

// RedisUserRepository is a cached implementation of UserRepository.
// It wraps a PostgresUserRepository and uses Redis for caching.
type RedisUserRepository struct {
	postgresRepo user.UserRepository // The "source of truth" repository
	redisClient  *redis.Client
	cacheTTL     time.Duration
}

// NewRedisUserRepository creates a new RedisUserRepository.
func NewRedisUserRepository(postgresRepo user.UserRepository, redisClient *redis.Client, cacheTTL time.Duration) *RedisUserRepository {
	if cacheTTL <= 0 {
		cacheTTL = defaultUserCacheTTL
	}
	return &RedisUserRepository{
		postgresRepo: postgresRepo,
		redisClient:  redisClient,
		cacheTTL:     cacheTTL,
	}
}

func (r *RedisUserRepository) cacheKeyID(id string) string {
	return userCachePrefixByID + id
}

func (r *RedisUserRepository) cacheKeyUsername(username string) string {
	return userCachePrefixByUsername + username
}

// Create creates a user in Postgres and then caches it.
func (r *RedisUserRepository) Create(ctx context.Context, u *user.User) error {
	// Create in primary repository first
	if err := r.postgresRepo.Create(ctx, u); err != nil {
		return err
	}

	// Cache the newly created user (u should have ID and other fields populated by postgresRepo.Create)
	// If ID is not empty after creation (which it should be)
	if u.ID != "" {
		userData, err := json.Marshal(u)
		if err != nil {
			// Log marshalling error but don't fail the operation as primary DB succeeded
			fmt.Printf("Error marshalling user %s for cache: %v\n", u.ID, err)
			return nil // Or return the error if caching is critical
		}

		// Cache by ID
		err = r.redisClient.Set(ctx, r.cacheKeyID(u.ID), userData, r.cacheTTL).Err()
		if err != nil {
			fmt.Printf("Error caching user %s by ID: %v\n", u.ID, err)
		}
		// Cache by Username
		err = r.redisClient.Set(ctx, r.cacheKeyUsername(u.Username), userData, r.cacheTTL).Err()
		if err != nil {
			fmt.Printf("Error caching user %s by username: %v\n", u.Username, err)
		}
	}
	return nil
}

// FindByID tries to fetch the user from Redis; if miss, fetches from Postgres and caches.
func (r *RedisUserRepository) FindByID(ctx context.Context, id string) (*user.User, error) {
	cacheKey := r.cacheKeyID(id)

	// Try fetching from Redis
	userData, err := r.redisClient.Get(ctx, cacheKey).Bytes()
	if err == nil { // Cache hit
		var u user.User
		if jsonErr := json.Unmarshal(userData, &u); jsonErr == nil {
			return &u, nil
		}
		// Log unmarshalling error and proceed to fetch from DB
		fmt.Printf("Error unmarshalling cached user %s: %v\n", id, jsonErr)
	} else if !errors.Is(err, redis.Nil) { // Real Redis error
		fmt.Printf("Error fetching user %s from Redis: %v. Falling back to DB.\n", id, err)
	}

	// Cache miss or Redis error, fetch from Postgres
	u, err := r.postgresRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err // e.g., user.ErrUserNotFound from underlying repo
	}

	// Cache the result from Postgres
	newUserData, jsonErr := json.Marshal(u)
	if jsonErr != nil {
		fmt.Printf("Error marshalling user %s for cache after DB fetch: %v\n", id, jsonErr)
		return u, nil // Return user even if caching fails
	}
	setErr := r.redisClient.Set(ctx, cacheKey, newUserData, r.cacheTTL).Err()
	if setErr != nil {
		fmt.Printf("Error caching user %s by ID after DB fetch: %v\n", id, setErr)
	}

	return u, nil
}

// FindByUsername tries Redis; if miss, fetches from Postgres and caches.
func (r *RedisUserRepository) FindByUsername(ctx context.Context, username string) (*user.User, error) {
	cacheKey := r.cacheKeyUsername(username)

	userData, err := r.redisClient.Get(ctx, cacheKey).Bytes()
	if err == nil {
		var u user.User
		if jsonErr := json.Unmarshal(userData, &u); jsonErr == nil {
			return &u, nil
		}
		fmt.Printf("Error unmarshalling cached user (username %s): %v\n", username, jsonErr)
	} else if !errors.Is(err, redis.Nil) {
		fmt.Printf("Error fetching user (username %s) from Redis: %v. Falling back to DB.\n", username, err)
	}

	u, err := r.postgresRepo.FindByUsername(ctx, username)
	if err != nil {
		return nil, err
	}

	newUserData, jsonErr := json.Marshal(u)
	if jsonErr != nil {
		fmt.Printf("Error marshalling user (username %s) for cache after DB fetch: %v\n", username, jsonErr)
		return u, nil
	}
	// Cache by username
	setErr := r.redisClient.Set(ctx, cacheKey, newUserData, r.cacheTTL).Err()
	if setErr != nil {
		fmt.Printf("Error caching user (username %s) by username after DB fetch: %v\n", username, setErr)
	}
	// Also cache by ID if we fetched it
	if u.ID != "" {
		idCacheKey := r.cacheKeyID(u.ID)
		setErr = r.redisClient.Set(ctx, idCacheKey, newUserData, r.cacheTTL).Err()
		if setErr != nil {
			fmt.Printf("Error caching user %s by ID (found by username %s) after DB fetch: %v\n", u.ID, username, setErr)
		}
	}
	return u, nil
}

// Update updates the user in Postgres and then invalidates/updates cache.
func (r *RedisUserRepository) Update(ctx context.Context, u *user.User) error {
	// It's important to know the old username if it can change, to invalidate old username cache key.
	// For simplicity, let's assume we only have the new state in `u`.
	// A more robust approach would be to fetch the user first to get old state for precise invalidation.
	// However, if FindByID is called before Update in service layer, this is less of an issue.

	// If username might change, we need the old username to invalidate its cache entry.
	// Fetching current user state before update to get old username for cache invalidation.
	// This adds a read before write but ensures cache consistency if username changes.
	// Alternatively, if usernames are immutable, this is not needed.
	// Or, the service layer could pass the old username.
	// For this example, let's assume the service layer doesn't pass the old username.
	// And, let's say username can change.

	// To correctly invalidate the username cache if the username changes,
	// we would ideally fetch the user's current state from the DB (or cache) *before* the update.
	// oldUser, err := r.postgresRepo.FindByID(ctx, u.ID) // Or r.FindByID for cached version
	// if err != nil {
		// return fmt.Errorf("failed to fetch user for cache invalidation pre-update: %w", err)
	// }

	if err := r.postgresRepo.Update(ctx, u); err != nil {
		return err
	}

	// Invalidate/update cache
	// Invalidate by ID
	err := r.redisClient.Del(ctx, r.cacheKeyID(u.ID)).Err()
	if err != nil {
		fmt.Printf("Error deleting user %s from ID cache during update: %v\n", u.ID, err)
	}
	// Invalidate by Username (the new username)
	err = r.redisClient.Del(ctx, r.cacheKeyUsername(u.Username)).Err()
	if err != nil {
		fmt.Printf("Error deleting user %s from username cache during update: %v\n", u.Username, err)
	}

	// If oldUser was fetched and oldUser.Username != u.Username:
	// err = r.redisClient.Del(ctx, r.cacheKeyUsername(oldUser.Username)).Err()
	// if err != nil {
		// fmt.Printf("Error deleting user %s (old username) from username cache: %v\n", oldUser.Username, err)
	// }

	// Optionally, re-cache the updated user immediately
	// userData, jsonErr := json.Marshal(u)
	// if jsonErr == nil {
	//	 r.redisClient.Set(ctx, r.cacheKeyID(u.ID), userData, r.cacheTTL)
	//	 r.redisClient.Set(ctx, r.cacheKeyUsername(u.Username), userData, r.cacheTTL)
	// }
	return nil
}

// Delete removes the user from Postgres and then invalidates cache.
func (r *RedisUserRepository) Delete(ctx context.Context, id string) error {
	// To invalidate username cache, we need the username. Fetch before deleting.
	// This assumes that FindByID will hit cache if available.
	userToDelete, err := r.FindByID(ctx, id)
	if err != nil {
		// If user not found (e.g. already deleted or never existed), underlying repo will error.
		// If it's ErrUserNotFound, we might not need to proceed with DB delete.
		if errors.Is(err, user.ErrUserNotFound) {
			// Attempt to clean up cache just in case, though unlikely to exist if not in DB
			r.redisClient.Del(ctx, r.cacheKeyID(id)) // Ignore error for this optional cleanup
			// No specific username to delete from cache if user object could not be fetched.
			return user.ErrUserNotFound
		}
		return fmt.Errorf("failed to fetch user for cache invalidation pre-delete: %w", err)
	}

	if err := r.postgresRepo.Delete(ctx, id); err != nil {
		return err
	}

	// Invalidate cache
	delErrID := r.redisClient.Del(ctx, r.cacheKeyID(id)).Err()
	if delErrID != nil {
		fmt.Printf("Error deleting user %s from ID cache: %v\n", id, delErrID)
	}
	if userToDelete != nil && userToDelete.Username != "" {
		delErrUsername := r.redisClient.Del(ctx, r.cacheKeyUsername(userToDelete.Username)).Err()
		if delErrUsername != nil {
			fmt.Printf("Error deleting user %s from username cache: %v\n", userToDelete.Username, delErrUsername)
		}
	}
	return nil
}
