package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"main/pkg/captioner"
	"main/pkg/captioner/gemini"
)


func CreateCaptioningService(config *CaptionAPIConfig) (captioner.Captioner, error) {
	if config == nil {
		return nil, fmt.Errorf("caption API configuration is required")
	}

	switch config.Provider {
	case "gemini":
		model := captioner.Model(config.Model)
		if model == "" {
			model = gemini.ModelGeminiPro
		}
		return gemini.NewGemini(config.APIKey, model, &http.Client{})
	default:
		return nil, fmt.Errorf("unsupported caption API provider: %s", config.Provider)
	}
}

func LoadImage(imagePath string) (*captioner.Image, error) {
	imageFile, err := os.Open(imagePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open image file: %v", err)
	}
	defer imageFile.Close()

	imageData, err := io.ReadAll(imageFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read image file: %v", err)
	}

	// Determine MIME type based on file extension
	mimeType := captioner.MimeTypeJPEG
	ext := filepath.Ext(imagePath)
	switch ext {
	case ".png":
		mimeType = captioner.MimeTypePNG
	case ".gif":
		mimeType = captioner.MimeTypeGIF
	case ".webp":
		mimeType = captioner.MimeTypeWebP
	}

	return captioner.NewImageFromBytes(imageData, mimeType)
}

func GenerateCaptionForTask(ctx context.Context, projectID, taskID string) (*CaptionResponse, error) {
	// Get the caption task
	task, err := getCaptionTask(taskID)
	if err != nil {
		return &CaptionResponse{Error: fmt.Sprintf("Failed to get caption task: %v", err)}, nil
	}
	if task == nil {
		return &CaptionResponse{Error: "Caption task not found"}, nil
	}

	// Get the project to access API configuration
	project, err := getProject(projectID)
	if err != nil {
		return &CaptionResponse{Error: fmt.Sprintf("Failed to get project: %v", err)}, nil
	}
	if project == nil {
		return &CaptionResponse{Error: "Project not found"}, nil
	}

	// Check if caption API is configured
	if project.CaptionAPI == nil {
		return &CaptionResponse{Error: "Caption API not configured for this project"}, nil
	}

	// Parse the caption API configuration
	var apiConfig CaptionAPIConfig
	if err := json.Unmarshal([]byte(*project.CaptionAPI), &apiConfig); err != nil {
		return &CaptionResponse{Error: fmt.Sprintf("Invalid caption API configuration: %v", err)}, nil
	}

	// Get the image
	image, err := getImage(task.ImageID)
	if err != nil {
		return &CaptionResponse{Error: fmt.Sprintf("Failed to get image: %v", err)}, nil
	}
	if image == nil {
		return &CaptionResponse{Error: "Image not found"}, nil
	}

	// Load image
	imagePath := filepath.Join("data", "projects", projectID, image.Path)
	captionerImage, err := LoadImage(imagePath)
	if err != nil {
		return &CaptionResponse{Error: fmt.Sprintf("Failed to load image: %v", err)}, nil
	}

	// Create captioning service
	captioningService, err := CreateCaptioningService(&apiConfig)
	if err != nil {
		return &CaptionResponse{Error: fmt.Sprintf("Failed to create captioning service: %v", err)}, nil
	}

	// Use system prompt from project or default
	systemPrompt := "Describe this image in detail for training a diffusion model. Focus on the visual elements, composition, style, and any notable features."
	if project.SystemPrompt != nil && *project.SystemPrompt != "" {
		systemPrompt = *project.SystemPrompt
	}

	// Generate caption
	caption, err := captioningService.CaptionSingle(ctx, *captionerImage, systemPrompt)
	if err != nil {
		logger.Error("Failed to generate caption", "error", err)
		return &CaptionResponse{Error: fmt.Sprintf("Failed to generate caption: %v", err)}, nil
	}

	// Update the task with the generated caption and set status to auto_generated
	task.Caption.String = caption
	task.Caption.Valid = true
	task.Status = "auto_generated"

	if err := updateCaptionTask(task); err != nil {
		logger.Error("Failed to update caption task with generated caption", "error", err)
		return &CaptionResponse{Error: fmt.Sprintf("Failed to save generated caption: %v", err)}, nil
	}

	return &CaptionResponse{Caption: caption}, nil
}
