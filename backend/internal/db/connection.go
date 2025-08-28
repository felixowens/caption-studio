// Package db provides database related functionality.
package db

import (
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	_ "github.com/mattn/go-sqlite3" // sqlite3 driver
)

var db *sql.DB

// InitDatabase initializes the database connection.
func InitDatabase(path string, logger *slog.Logger) (*sql.DB, error) {
	// Open database connection
	var err error
	db, err = sql.Open("sqlite3", path+"?_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %v", err)
	}

	// Set connection pool settings
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Test connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %v", err)
	}

	logger.Info("Database initialized successfully",
		"db_path", path,
		"max_connections", 25,
	)
	return db, nil
}
