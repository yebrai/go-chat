package cache

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
)

const (
	// roomMessagesPrefix is the Redis key prefix for storing recent messages for a room.
	// Format: room:<roomID>:messages
	roomMessagesPrefix = "room:%s:messages"

	// roomUsersPrefix is the Redis key prefix for storing active users in a room.
	// Format: room:<roomID>:users
	roomUsersPrefix = "room:%s:users"

	// roomMessageCountPrefix is the Redis key prefix for storing the total message count for a room.
	// Format: room:<roomID>:message_count
	roomMessageCountPrefix = "room:%s:message_count"

	// globalUsersSetKey is the Redis key for the global set of all active users.
	globalUsersSetKey = "global:users"

	// Default TTL for a room's user set if no users are active, making the room entry ephemeral.
	// This helps in cleaning up empty/inactive room user sets from Redis.
	defaultRoomUserSetTTL = 2 * time.Hour

	// Default TTL for the list of recent messages for a room.
	// Helps in managing memory for inactive rooms.
	defaultRecentMessagesListTTL = 24 * time.Hour
)

// RedisClient wraps the go-redis client, providing chat-specific caching operations.
type RedisClient struct {
	client *redis.Client
}

// NewRedisClient creates and returns a new RedisClient.
// It takes a redisURL string (e.g., "redis://localhost:6379/0"),
// parses it, creates a new Redis client instance, and pings the server
// to verify the connection.
func NewRedisClient(redisURL string) (*RedisClient, error) {
	if redisURL == "" {
		return nil, fmt.Errorf("redis URL cannot be empty")
	}

	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse redis URL '%s': %w", redisURL, err)
	}

	client := redis.NewClient(opts)

	// Ping the Redis server to ensure connectivity.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = client.Ping(ctx).Result()
	if err != nil {
		client.Close() // Close client if ping fails.
		return nil, fmt.Errorf("failed to ping redis at '%s': %w", redisURL, err)
	}

	return &RedisClient{client: client}, nil
}

// --- Message Operations ---

// AddRecentMessage adds a message (as a JSON string) to a room's recent message list in Redis.
// It uses LPUSH to add to the head of the list and LTRIM to keep the list at maxMessages.
// If messageTTL is greater than 0, it sets an EXPIRE on the list key.
func (rc *RedisClient) AddRecentMessage(ctx context.Context, roomID string, messageJSON string, maxMessages int, messageTTL time.Duration) error {
	if roomID == "" {
		return fmt.Errorf("roomID cannot be empty")
	}
	if messageJSON == "" {
		return fmt.Errorf("messageJSON cannot be empty")
	}
	listKey := fmt.Sprintf(roomMessagesPrefix, roomID)

	// Use a pipeline for atomic execution of commands.
	pipe := rc.client.TxPipeline()
	pipe.LPush(ctx, listKey, messageJSON)
	if maxMessages > 0 {
		// LTRIM keeps the list from index 0 to maxMessages-1 (inclusive).
		pipe.LTrim(ctx, listKey, 0, int64(maxMessages-1))
	}
	if messageTTL > 0 {
		pipe.Expire(ctx, listKey, messageTTL)
	} else { // Default TTL if not specified, to prevent keys from living forever
		pipe.Expire(ctx, listKey, defaultRecentMessagesListTTL)
	}


	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to execute Redis pipeline for adding recent message to room '%s': %w", roomID, err)
	}
	return nil
}

// GetRecentMessages retrieves a specified number of recent messages (JSON strings) for a room from Redis.
// It uses LRANGE to get messages from the head of the list.
// If count is invalid (<=0), it defaults to 10.
// Returns an empty slice if the room has no messages or the key doesn't exist.
func (rc *RedisClient) GetRecentMessages(ctx context.Context, roomID string, count int) ([]string, error) {
	if roomID == "" {
		return nil, fmt.Errorf("roomID cannot be empty")
	}
	if count <= 0 {
		count = 10 // Default count if an invalid value is provided.
	}
	listKey := fmt.Sprintf(roomMessagesPrefix, roomID)

	messages, err := rc.client.LRange(ctx, listKey, 0, int64(count-1)).Result()
	if err != nil {
		if err == redis.Nil {
			return []string{}, nil // Key doesn't exist, which is not an error, just no messages.
		}
		return nil, fmt.Errorf("failed to get recent messages for room '%s' from Redis: %w", roomID, err)
	}
	return messages, nil
}

// --- User Operations ---

// AddActiveUserToRoom adds a username to a room's active user set in Redis.
// If userTTL is greater than 0, it sets an EXPIRE on the room's user set key.
// This helps in making the user set ephemeral if the room becomes inactive.
func (rc *RedisClient) AddActiveUserToRoom(ctx context.Context, roomID string, username string, userTTL time.Duration) error {
	if roomID == "" || username == "" {
		return fmt.Errorf("roomID and username cannot be empty")
	}
	setKey := fmt.Sprintf(roomUsersPrefix, roomID)

	pipe := rc.client.TxPipeline()
	pipe.SAdd(ctx, setKey, username)
	if userTTL > 0 {
		pipe.Expire(ctx, setKey, userTTL)
	} else { // Default TTL for the room user set
		pipe.Expire(ctx, setKey, defaultRoomUserSetTTL)
	}

	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to execute Redis pipeline for adding user '%s' to room '%s': %w", username, roomID, err)
	}
	return nil
}

// RemoveActiveUserFromRoom removes a username from a room's active user set in Redis.
func (rc *RedisClient) RemoveActiveUserFromRoom(ctx context.Context, roomID string, username string) error {
	if roomID == "" || username == "" {
		return fmt.Errorf("roomID and username cannot be empty")
	}
	setKey := fmt.Sprintf(roomUsersPrefix, roomID)

	err := rc.client.SRem(ctx, setKey, username).Err()
	// SRem does not return redis.Nil if the key does not exist or member not in set.
	// It returns the number of members removed (0 or 1).
	if err != nil {
		return fmt.Errorf("failed to remove user '%s' from room '%s' set in Redis: %w", username, roomID, err)
	}
	return nil
}

// GetActiveUsersInRoom retrieves all usernames from a room's active user set in Redis.
// Returns an empty slice if the room has no active users or the key doesn't exist.
func (rc *RedisClient) GetActiveUsersInRoom(ctx context.Context, roomID string) ([]string, error) {
	if roomID == "" {
		return nil, fmt.Errorf("roomID cannot be empty")
	}
	setKey := fmt.Sprintf(roomUsersPrefix, roomID)

	users, err := rc.client.SMembers(ctx, setKey).Result()
	if err != nil {
		// SMembers returns redis.Nil if the key does not exist, though the go-redis client might translate this.
		// More commonly, it returns an empty list and no error if key doesn't exist.
		// Let's be safe and check for redis.Nil explicitly if the library guarantees it.
		// However, current go-redis versions typically return empty slice and no error for non-existent sets.
		return nil, fmt.Errorf("failed to get active users for room '%s' from Redis: %w", roomID, err)
	}
	return users, nil
}

// AddUserToGlobalSet adds a username to the global set of all currently active users.
// This set does not have a TTL; users are removed explicitly on unregistration/disconnect.
func (rc *RedisClient) AddUserToGlobalSet(ctx context.Context, username string) error {
	if username == "" {
		return fmt.Errorf("username cannot be empty")
	}
	err := rc.client.SAdd(ctx, globalUsersSetKey, username).Err()
	if err != nil {
		return fmt.Errorf("failed to add user '%s' to global set in Redis: %w", username, err)
	}
	return nil
}

// RemoveUserFromGlobalSet removes a username from the global set of active users.
func (rc *RedisClient) RemoveUserFromGlobalSet(ctx context.Context, username string) error {
	if username == "" {
		return fmt.Errorf("username cannot be empty")
	}
	err := rc.client.SRem(ctx, globalUsersSetKey, username).Err()
	if err != nil {
		return fmt.Errorf("failed to remove user '%s' from global set in Redis: %w", username, err)
	}
	return nil
}

// GetGlobalActiveUserCount retrieves the total number of users in the global active set.
func (rc *RedisClient) GetGlobalActiveUserCount(ctx context.Context) (int64, error) {
	count, err := rc.client.SCard(ctx, globalUsersSetKey).Result()
	// SCard returns 0 if the key does not exist, not redis.Nil.
	if err != nil {
		return 0, fmt.Errorf("failed to get global active user count from Redis: %w", err)
	}
	return count, nil
}

// --- Counter Operations ---

// IncrementMessageCounter increments the total message count for a specific room in Redis.
// Returns the new count after incrementing.
func (rc *RedisClient) IncrementMessageCounter(ctx context.Context, roomID string) (int64, error) {
	if roomID == "" {
		return 0, fmt.Errorf("roomID cannot be empty")
	}
	counterKey := fmt.Sprintf(roomMessageCountPrefix, roomID)

	newCount, err := rc.client.Incr(ctx, counterKey).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to increment message counter for room '%s' in Redis: %w", roomID, err)
	}
	return newCount, nil
}

// GetRoomMessageCount retrieves the total message count for a specific room from Redis.
// Returns 0 if the counter key does not exist (i.e., no messages counted yet for the room).
func (rc *RedisClient) GetRoomMessageCount(ctx context.Context, roomID string) (int64, error) {
	if roomID == "" {
		return 0, fmt.Errorf("roomID cannot be empty")
	}
	counterKey := fmt.Sprintf(roomMessageCountPrefix, roomID)

	val, err := rc.client.Get(ctx, counterKey).Result()
	if err != nil {
		if err == redis.Nil {
			return 0, nil // Key doesn't exist, so count is 0. This is expected.
		}
		return 0, fmt.Errorf("failed to get message count for room '%s' from Redis: %w", roomID, err)
	}

	count, convErr := strconv.ParseInt(val, 10, 64)
	if convErr != nil {
		// This case indicates corrupted data in Redis if the value is not a valid integer.
		return 0, fmt.Errorf("failed to parse message count '%s' for room '%s' from Redis: %w", val, roomID, convErr)
	}
	return count, nil
}

// --- Stats Operation ---

// GetRoomStats retrieves a map of statistics for a room from Redis.
// This includes the count of active users in the room and the total message count for the room.
func (rc *RedisClient) GetRoomStats(ctx context.Context, roomID string) (map[string]int64, error) {
	if roomID == "" {
		return nil, fmt.Errorf("roomID cannot be empty")
	}
	stats := make(map[string]int64)

	// Get active user count in room
	userSetKey := fmt.Sprintf(roomUsersPrefix, roomID)
	activeUsers, err := rc.client.SCard(ctx, userSetKey).Result()
	// SCard returns 0 if key doesn't exist, not an error typically.
	if err != nil {
		return nil, fmt.Errorf("failed to get active user count for room '%s' stats from Redis: %w", roomID, err)
	}
	stats["active_users"] = activeUsers

	// Get message count for room
	messageCount, err := rc.GetRoomMessageCount(ctx, roomID) // This handles redis.Nil correctly.
	if err != nil {
		// Error from GetRoomMessageCount would already be descriptive.
		return nil, fmt.Errorf("failed to get message count for room '%s' stats: %w", roomID, err)
	}
	stats["message_count"] = messageCount

	return stats, nil
}

// Close gracefully closes the Redis client connection.
// It should be called when the application shuts down.
func (rc *RedisClient) Close() error {
	if rc.client != nil {
		return rc.client.Close()
	}
	return nil
}
