// Package service provides the service layer for the application.
package service

import (
	"log/slog"
	"main/internal/db"
)

// Service is the service layer for the application.
type Service struct {
	db     *db.Queries
	logger *slog.Logger
}

// NewService creates a new validated Service.
func NewService(db *db.Queries, logger *slog.Logger) *Service {
	return &Service{db: db, logger: logger}
}
