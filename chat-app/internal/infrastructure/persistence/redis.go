package persistence

import (
	"context"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
)

// NewRedisClient establishes a new connection to a Redis server.
func NewRedisClient(redisURL string) (*redis.Client, error) {
	if redisURL == "" {
		return nil, fmt.Errorf("redis URL cannot be empty")
	}

	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse redis URL: %w", err)
	}

	// It's good practice to have a context for the initial Ping.
	// Using a short timeout for the connection attempt.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := redis.NewClient(opts)

	// Verify the connection by pinging the Redis server.
	_, err = client.Ping(ctx).Result()
	if err != nil {
		client.Close() // Close the client if Ping fails.
		return nil, fmt.Errorf("failed to ping redis: %w", err)
	}

	fmt.Println("Successfully connected to Redis server.")
	return client, nil
}
