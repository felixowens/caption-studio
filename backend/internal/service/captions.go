package service

import (
	"log/slog"
	"main/internal/db"
)

// ImageCaptioner TODO:
type ImageCaptioner interface {
	// TODO: temp interface
	CaptionImage() (string, error)
}

// CaptionService TODO:
type CaptionService interface {
	ImageCaptioner
}

type captionService struct {
	db     *db.Queries
	logger *slog.Logger
}

// NewCaptionService creates a new caption service.
func NewCaptionService(db *db.Queries, logger *slog.Logger) CaptionService {
	return &captionService{db: db, logger: logger}
}

// Foo is a placeholder for a method.
func (s *captionService) Foo() error {
	return nil
}

// CaptionImage will caption an image.
func (s *captionService) CaptionImage() (string, error) {
	return "Hello, world!", nil
}
