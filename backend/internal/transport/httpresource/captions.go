// Package httpresource provides the HTTP resource layer for the application.
package httpresource

import (
	"context"
	"log/slog"
	"main/internal/config"
	"main/internal/service"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

const (
	maxBodyBytes = 1024 * 1024 * 10 // 10MB
)

// CaptionsResource is the resource for the captions resource.
type CaptionsResource struct {
	service service.CaptionService
	config  config.Config
	logger  *slog.Logger
}

// ProvisionCaptionsResource will provision a HTTP handler for the captions resource.
func ProvisionCaptionsResource(
	api huma.API,
	config config.Config,
	service service.CaptionService,
	logger *slog.Logger,
) {
	resource := CaptionsResource{
		config:  config,
		service: service,
		logger:  logger,
	}

	huma.Register(api, huma.Operation{
		OperationID:   "captionImage",
		Tags:          []string{"captions"},
		Summary:       "Caption an image",
		Description:   "Creates a caption for an image.",
		Method:        http.MethodPost,
		Path:          "/captions",
		MaxBodyBytes:  maxBodyBytes,
		DefaultStatus: http.StatusCreated,
	}, resource.CaptionImage)
}

// PostCaptionsRequest is the request for the post captions request.
type PostCaptionsRequest struct {
	Body struct {
		ImageID string `json:"image_id" format:"uuid" example:"123e4567-e89b-12d3-a456-426614174000" doc:"The ID of the image to caption."`
	} `json:"body"`
}

// PostCaptionsResponse is the response for the post captions request.
type PostCaptionsResponse struct {
	Body struct {
		Caption string `json:"caption" example:"A beautiful sunset over the ocean."`
	} `json:"body"`
}

// CaptionImage will caption an image.
func (r *CaptionsResource) CaptionImage(_ context.Context, input *PostCaptionsRequest) (*PostCaptionsResponse, error) {
	if input.Body.ImageID == "" {
		return nil, huma.Error400BadRequest("image_id is required")
	}

	r.logger.Info("Captioning image", "image_id", input.Body.ImageID)
	caption, err := r.service.CaptionImage()
	if err != nil {
		return nil, err
	}

	return &PostCaptionsResponse{
		Body: struct {
			Caption string `json:"caption" example:"A beautiful sunset over the ocean."`
		}{Caption: caption},
	}, nil
}
