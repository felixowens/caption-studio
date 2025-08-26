package main

import (
	"database/sql"
	"time"
)

type Project struct {
	ID                 string    `json:"id" db:"id"`
	Name               string    `json:"name" db:"name"`
	Version            string    `json:"version" db:"version"`
	PromptButtons      []string  `json:"promptButtons" db:"prompt_buttons"`
	ParentProjectID    *string   `json:"parentProjectId" db:"parent_project_id"`
	ProjectType        string    `json:"projectType" db:"project_type"` // "edit" or "caption"
	CaptionAPI         *string   `json:"captionApi" db:"caption_api"`   // JSON configuration for caption API
	SystemPrompt       *string   `json:"systemPrompt" db:"system_prompt"` // Custom system prompt for captioning
	AutoCaptionConfig  *string   `json:"autoCaptionConfig" db:"auto_caption_config"` // JSON configuration for auto captioning
	CreatedAt          time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt          time.Time `json:"updatedAt" db:"updated_at"`
}

type Image struct {
	ID        string    `json:"id" db:"id"`
	ProjectID string    `json:"projectId" db:"project_id"`
	Path      string    `json:"path" db:"path"`
	PHash     string    `json:"pHash" db:"phash"`
	CreatedAt time.Time `json:"createdAt" db:"created_at"`
}

type Task struct {
	ID            string         `json:"id" db:"id"`
	ProjectID     string         `json:"projectId" db:"project_id"`
	ImageAID      string         `json:"imageAId" db:"image_a_id"`
	ImageBId      sql.NullString `json:"imageBId" db:"image_b_id"`
	Prompt        sql.NullString `json:"prompt" db:"prompt"`
	Status        string         `json:"status" db:"status"` // "pending", "auto_generated", "reviewed", "completed"
	Skipped       bool           `json:"skipped" db:"skipped"`
	CandidateBIds []string       `json:"candidateBIds"`
	CreatedAt     time.Time      `json:"createdAt" db:"created_at"`
	UpdatedAt     time.Time      `json:"updatedAt" db:"updated_at"`
}

type CaptionTask struct {
	ID          string         `json:"id" db:"id"`
	ProjectID   string         `json:"projectId" db:"project_id"`
	ImageID     string         `json:"imageId" db:"image_id"`
	Caption     sql.NullString `json:"caption" db:"caption"`
	Status      string         `json:"status" db:"status"` // "pending", "auto_generated", "reviewed", "completed"
	Skipped     bool           `json:"skipped" db:"skipped"`
	CreatedAt   time.Time      `json:"createdAt" db:"created_at"`
	UpdatedAt   time.Time      `json:"updatedAt" db:"updated_at"`
}

type CaptionAPIConfig struct {
	Provider string `json:"provider"` // "gemini", "openai", etc.
	APIKey   string `json:"apiKey"`
	Endpoint string `json:"endpoint,omitempty"`
	Model    string `json:"model,omitempty"`
}

type CaptionRequest struct {
	ImageBase64  string `json:"imageBase64"`
	SystemPrompt string `json:"systemPrompt"`
}

type CaptionResponse struct {
	Caption string `json:"caption"`
	Error   string `json:"error,omitempty"`
}

type AutoCaptionConfig struct {
	RPM              int    `json:"rpm"`              // Requests per minute
	MaxRetries       int    `json:"maxRetries"`       // Maximum retry attempts
	RetryDelayMs     int    `json:"retryDelayMs"`     // Base retry delay in milliseconds
	ConcurrentTasks  int    `json:"concurrentTasks"`  // Number of concurrent processing tasks
}

type AutoCaptionProgress struct {
	ProjectID    string `json:"projectId"`
	Status       string `json:"status"`       // "running", "completed", "cancelled", "error"
	Total        int    `json:"total"`
	Processed    int    `json:"processed"`
	Successful   int    `json:"successful"`
	Failed       int    `json:"failed"`
	CurrentTask  string `json:"currentTask,omitempty"`
	ErrorMessage string `json:"errorMessage,omitempty"`
	StartedAt    string `json:"startedAt,omitempty"`
	CompletedAt  string `json:"completedAt,omitempty"`
}

type AutoCaptionRequest struct {
	Config AutoCaptionConfig `json:"config"`
}

type AutoCaptionStatusResponse struct {
	Progress *AutoCaptionProgress `json:"progress"`
	IsActive bool                 `json:"isActive"`
}