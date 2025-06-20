package persistence

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq" // PostgreSQL driver
)

const (
	DefaultMaxOpenConns    = 25
	DefaultMaxIdleConns    = 5
	DefaultConnMaxLifetime = 5 * time.Minute
	DefaultConnMaxIdleTime = 2 * time.Minute
)

// NewPostgresDB establishes a new connection to a PostgreSQL database.
// It configures connection pooling and verifies the connection.
func NewPostgresDB(dsn string) (*sql.DB, error) {
	if dsn == "" {
		return nil, fmt.Errorf("database DSN cannot be empty")
	}

	// Open a new database connection.
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %w", err)
	}

	// Configure connection pool settings.
	db.SetMaxOpenConns(DefaultMaxOpenConns)
	db.SetMaxIdleConns(DefaultMaxIdleConns)
	db.SetConnMaxLifetime(DefaultConnMaxLifetime)
	db.SetConnMaxIdleTime(DefaultConnMaxIdleTime)

	// Verify the connection by pinging the database.
	// It's good practice to use a context for the Ping if your app uses contexts heavily,
	// but for initialization, a simple Ping is often sufficient.
	// For longer-running apps, a retry mechanism might be useful here.
	err = db.Ping()
	if err != nil {
		db.Close() // Close the connection if Ping fails.
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	fmt.Println("Successfully connected to PostgreSQL database.")
	return db, nil
}
