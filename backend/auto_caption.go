package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"main/pkg/captioner"
)

// AutoCaptionManager handles bulk auto captioning with rate limiting
type AutoCaptionManager struct {
	mutex               sync.RWMutex
	activeProjects      map[string]*AutoCaptionSession
	progressClients     map[string]chan AutoCaptionProgress
	progressClientsMu   sync.RWMutex
}

// AutoCaptionSession represents an active auto captioning session
type AutoCaptionSession struct {
	ProjectID       string
	ProjectType     string // "edit" or "caption"
	Config          AutoCaptionConfig
	Progress        AutoCaptionProgress
	CancelFunc      context.CancelFunc
	CaptionTasks    []CaptionTask // for caption projects
	EditTasks       []Task        // for edit projects
	CurrentIndex    int
	mutex           sync.RWMutex
}

var autoCaptionManager *AutoCaptionManager

func init() {
	autoCaptionManager = &AutoCaptionManager{
		activeProjects:  make(map[string]*AutoCaptionSession),
		progressClients: make(map[string]chan AutoCaptionProgress),
	}
}

// StartAutoCaptioning begins the auto captioning process for a project
func (acm *AutoCaptionManager) StartAutoCaptioning(projectID string, config AutoCaptionConfig) error {
	acm.mutex.Lock()
	defer acm.mutex.Unlock()

	// Check if already running
	if _, exists := acm.activeProjects[projectID]; exists {
		return fmt.Errorf("auto captioning already running for project %s", projectID)
	}

	// Get project to validate and check API configuration
	project, err := getProject(projectID)
	if err != nil {
		return fmt.Errorf("failed to get project: %v", err)
	}
	if project == nil {
		return fmt.Errorf("project not found")
	}

	// Check if caption API is configured
	if project.CaptionAPI == nil {
		return fmt.Errorf("caption API not configured for this project")
	}

	// Handle different project types
	var totalPendingTasks int
	if project.ProjectType == "edit" {
		// For edit projects, get edit tasks
		editTasks, err := getTasksByProjectID(projectID)
		if err != nil {
			return fmt.Errorf("failed to get edit tasks: %v", err)
		}
		
		// Filter to pairs that have Image B selected but no prompt yet (allows manual pair selection + auto prompting)
		for _, task := range editTasks {
			if task.ImageBId.Valid && (!task.Prompt.Valid || task.Prompt.String == "") && !task.Skipped {
				totalPendingTasks++
			}
		}
		
		if totalPendingTasks == 0 {
			return fmt.Errorf("no pairs ready for auto captioning (pairs need Image B selected and empty prompts)")
		}
	} else {
		// For caption projects, get caption tasks
		allTasks, err := getCaptionTasksByProjectID(projectID)
		if err != nil {
			return fmt.Errorf("failed to get caption tasks: %v", err)
		}

		// Filter to pending tasks only
		for _, task := range allTasks {
			if task.Status == "pending" && !task.Skipped {
				totalPendingTasks++
			}
		}

		if totalPendingTasks == 0 {
			return fmt.Errorf("no pending tasks found for auto captioning")
		}
	}

	// Create session context
	ctx, cancel := context.WithCancel(context.Background())

	// Initialize session
	session := &AutoCaptionSession{
		ProjectID:   projectID,
		ProjectType: project.ProjectType,
		Config:      config,
		CancelFunc:  cancel,
		Progress: AutoCaptionProgress{
			ProjectID: projectID,
			Status:    "running",
			Total:     totalPendingTasks,
			StartedAt: time.Now().Format(time.RFC3339),
		},
	}

	// Populate task arrays based on project type
	if project.ProjectType == "edit" {
		editTasks, err := getTasksByProjectID(projectID)
		if err != nil {
			cancel()
			return fmt.Errorf("failed to get edit tasks: %v", err)
		}
		
		// Filter to edit tasks with Image B selected but no prompt yet
		var pendingEditTasks []Task
		for _, task := range editTasks {
			if task.ImageBId.Valid && (!task.Prompt.Valid || task.Prompt.String == "") && !task.Skipped {
				pendingEditTasks = append(pendingEditTasks, task)
			}
		}
		session.EditTasks = pendingEditTasks
	} else {
		allCaptionTasks, err := getCaptionTasksByProjectID(projectID)
		if err != nil {
			cancel()
			return fmt.Errorf("failed to get caption tasks: %v", err)
		}
		
		// Filter to pending caption tasks only
		var pendingCaptionTasks []CaptionTask
		for _, task := range allCaptionTasks {
			if task.Status == "pending" && !task.Skipped {
				pendingCaptionTasks = append(pendingCaptionTasks, task)
			}
		}
		session.CaptionTasks = pendingCaptionTasks
	}

	acm.activeProjects[projectID] = session

	// Start processing in background
	go acm.processAutoCaptioning(ctx, session, project)

	return nil
}

// CancelAutoCaptioning stops the auto captioning process for a project
func (acm *AutoCaptionManager) CancelAutoCaptioning(projectID string) error {
	acm.mutex.Lock()
	defer acm.mutex.Unlock()

	session, exists := acm.activeProjects[projectID]
	if !exists {
		return fmt.Errorf("no active auto captioning session for project %s", projectID)
	}

	session.CancelFunc()
	session.mutex.Lock()
	session.Progress.Status = "cancelled"
	session.Progress.CompletedAt = time.Now().Format(time.RFC3339)
	session.mutex.Unlock()

	// Send final progress update
	acm.sendProgressUpdate(projectID, session.Progress)

	delete(acm.activeProjects, projectID)
	return nil
}

// GetAutoCaptionStatus returns the current status of auto captioning for a project
func (acm *AutoCaptionManager) GetAutoCaptionStatus(projectID string) (*AutoCaptionStatusResponse, error) {
	acm.mutex.RLock()
	defer acm.mutex.RUnlock()

	session, exists := acm.activeProjects[projectID]
	if !exists {
		return &AutoCaptionStatusResponse{
			IsActive: false,
		}, nil
	}

	session.mutex.RLock()
	progress := session.Progress
	session.mutex.RUnlock()

	return &AutoCaptionStatusResponse{
		Progress: &progress,
		IsActive: true,
	}, nil
}

// processAutoCaptioning handles the actual captioning process
func (acm *AutoCaptionManager) processAutoCaptioning(ctx context.Context, session *AutoCaptionSession, project *Project) {
	defer func() {
		acm.mutex.Lock()
		delete(acm.activeProjects, session.ProjectID)
		acm.mutex.Unlock()
	}()

	// Parse caption API config
	var apiConfig CaptionAPIConfig
	if err := json.Unmarshal([]byte(*project.CaptionAPI), &apiConfig); err != nil {
		acm.updateProgress(session, "error", fmt.Sprintf("Invalid caption API configuration: %v", err))
		return
	}

	// Create captioning service
	captioningService, err := CreateCaptioningService(&apiConfig)
	if err != nil {
		acm.updateProgress(session, "error", fmt.Sprintf("Failed to create captioning service: %v", err))
		return
	}

	// Calculate delay between requests based on RPM
	requestDelay := time.Duration(60000/session.Config.RPM) * time.Millisecond

	// Get system prompt based on project type
	var systemPrompt string
	if session.ProjectType == "edit" {
		systemPrompt = "Compare these two images and describe the edit or transformation that was applied to convert the first image into the second image. Focus on the specific changes made, including any adjustments to color, lighting, objects, text, style, or composition. Be concise and descriptive."
	} else {
		systemPrompt = "Describe this image in detail for training a diffusion model. Focus on the visual elements, composition, style, and any notable features."
	}
	
	// Use custom system prompt if provided
	if project.SystemPrompt != nil && *project.SystemPrompt != "" {
		systemPrompt = *project.SystemPrompt
	}

	// Process tasks based on project type
	if session.ProjectType == "edit" {
		// Process edit tasks
		for i, task := range session.EditTasks {
			select {
			case <-ctx.Done():
				return
			default:
			}

			session.mutex.Lock()
			session.CurrentIndex = i
			session.Progress.CurrentTask = task.ID
			session.Progress.Processed = i
			session.mutex.Unlock()

			acm.sendProgressUpdate(session.ProjectID, session.Progress)

			// Process edit task with retries
			success := acm.processEditTaskWithRetries(ctx, task, session, captioningService, systemPrompt, project.ID)
			
			session.mutex.Lock()
			if success {
				session.Progress.Successful++
			} else {
				session.Progress.Failed++
			}
			session.mutex.Unlock()

			// Apply rate limiting delay (except for last task)
			if i < len(session.EditTasks)-1 {
				select {
				case <-ctx.Done():
					return
				case <-time.After(requestDelay):
				}
			}
		}
	} else {
		// Process caption tasks (existing logic)
		for i, task := range session.CaptionTasks {
			select {
			case <-ctx.Done():
				return
			default:
			}

			session.mutex.Lock()
			session.CurrentIndex = i
			session.Progress.CurrentTask = task.ID
			session.Progress.Processed = i
			session.mutex.Unlock()

			acm.sendProgressUpdate(session.ProjectID, session.Progress)

			// Process caption task with retries
			success := acm.processCaptionTaskWithRetries(ctx, task, session, captioningService, systemPrompt, project.ID)
			
			session.mutex.Lock()
			if success {
				session.Progress.Successful++
			} else {
				session.Progress.Failed++
			}
			session.mutex.Unlock()

			// Apply rate limiting delay (except for last task)
			if i < len(session.CaptionTasks)-1 {
				select {
				case <-ctx.Done():
					return
				case <-time.After(requestDelay):
				}
			}
		}
	}

	// Mark as completed
	session.mutex.Lock()
	session.Progress.Status = "completed"
	if session.ProjectType == "edit" {
		session.Progress.Processed = len(session.EditTasks)
	} else {
		session.Progress.Processed = len(session.CaptionTasks)
	}
	session.Progress.CurrentTask = ""
	session.Progress.CompletedAt = time.Now().Format(time.RFC3339)
	finalProgress := session.Progress
	session.mutex.Unlock()

	acm.sendProgressUpdate(session.ProjectID, finalProgress)
}

// processCaptionTaskWithRetries handles a single caption task with retry logic
func (acm *AutoCaptionManager) processCaptionTaskWithRetries(ctx context.Context, task CaptionTask, session *AutoCaptionSession, service captioner.Captioner, systemPrompt, projectID string) bool {
	maxRetries := session.Config.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}

	baseDelay := time.Duration(session.Config.RetryDelayMs) * time.Millisecond
	if baseDelay <= 0 {
		baseDelay = 1000 * time.Millisecond
	}

	for attempt := 0; attempt <= maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return false
		default:
		}

		// Get image
		image, err := getImage(task.ImageID)
		if err != nil {
			logger.Error("Failed to get image for auto captioning", "error", err, "task_id", task.ID)
			if attempt == maxRetries {
				return false
			}
			time.Sleep(baseDelay * time.Duration(attempt+1))
			continue
		}

		if image == nil {
			logger.Error("Image not found for auto captioning", "task_id", task.ID, "image_id", task.ImageID)
			return false
		}

		// Load image
		imagePath := filepath.Join("data", "projects", projectID, image.Path)
		captionerImage, err := LoadImage(imagePath)
		if err != nil {
			logger.Error("Failed to load image for auto captioning", "error", err, "path", imagePath)
			if attempt == maxRetries {
				return false
			}
			time.Sleep(baseDelay * time.Duration(attempt+1))
			continue
		}

		// Generate caption
		caption, err := service.CaptionSingle(ctx, *captionerImage, systemPrompt)
		if err != nil {
			logger.Error("Failed to generate caption", "error", err, "task_id", task.ID, "attempt", attempt+1)
			if attempt == maxRetries {
				return false
			}
			time.Sleep(baseDelay * time.Duration(attempt+1))
			continue
		}

		// Update task in database
		task.Caption.String = caption
		task.Caption.Valid = true
		task.Status = "auto_generated"

		if err := updateCaptionTask(&task); err != nil {
			logger.Error("Failed to update caption task", "error", err, "task_id", task.ID)
			if attempt == maxRetries {
				return false
			}
			time.Sleep(baseDelay * time.Duration(attempt+1))
			continue
		}

		logger.Info("Successfully generated auto caption", "task_id", task.ID, "caption_length", len(caption))
		return true
	}

	return false
}

// processEditTaskWithRetries handles a single edit task with retry logic
func (acm *AutoCaptionManager) processEditTaskWithRetries(ctx context.Context, task Task, session *AutoCaptionSession, service captioner.Captioner, systemPrompt, projectID string) bool {
	maxRetries := session.Config.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}

	baseDelay := time.Duration(session.Config.RetryDelayMs) * time.Millisecond
	if baseDelay <= 0 {
		baseDelay = 1000 * time.Millisecond
	}

	for attempt := 0; attempt <= maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return false
		default:
		}

		// Get image A
		imageA, err := getImage(task.ImageAID)
		if err != nil {
			logger.Error("Failed to get image A for auto captioning", "error", err, "task_id", task.ID, "image_a_id", task.ImageAID)
			if attempt == maxRetries {
				return false
			}
			time.Sleep(baseDelay * time.Duration(attempt+1))
			continue
		}
		if imageA == nil {
			logger.Error("Image A not found for auto captioning", "task_id", task.ID, "image_a_id", task.ImageAID)
			return false
		}

		// Get image B
		imageB, err := getImage(task.ImageBId.String)
		if err != nil {
			logger.Error("Failed to get image B for auto captioning", "error", err, "task_id", task.ID, "image_b_id", task.ImageBId.String)
			if attempt == maxRetries {
				return false
			}
			time.Sleep(baseDelay * time.Duration(attempt+1))
			continue
		}
		if imageB == nil {
			logger.Error("Image B not found for auto captioning", "task_id", task.ID, "image_b_id", task.ImageBId.String)
			return false
		}

		// Load images
		imageAPath := filepath.Join("data", "projects", projectID, imageA.Path)
		captionerImageA, err := LoadImage(imageAPath)
		if err != nil {
			logger.Error("Failed to load image A for auto captioning", "error", err, "path", imageAPath)
			if attempt == maxRetries {
				return false
			}
			time.Sleep(baseDelay * time.Duration(attempt+1))
			continue
		}

		imageBPath := filepath.Join("data", "projects", projectID, imageB.Path)
		captionerImageB, err := LoadImage(imageBPath)
		if err != nil {
			logger.Error("Failed to load image B for auto captioning", "error", err, "path", imageBPath)
			if attempt == maxRetries {
				return false
			}
			time.Sleep(baseDelay * time.Duration(attempt+1))
			continue
		}

		// Generate edit caption
		prompt, err := service.CaptionEdit(ctx, *captionerImageA, *captionerImageB, systemPrompt)
		if err != nil {
			logger.Error("Failed to generate edit caption", "error", err, "task_id", task.ID, "attempt", attempt+1)
			if attempt == maxRetries {
				return false
			}
			time.Sleep(baseDelay * time.Duration(attempt+1))
			continue
		}

		// Update task in database
		task.Prompt.String = prompt
		task.Prompt.Valid = true
		task.Status = "auto_generated"

		if err := updateTask(&task); err != nil {
			logger.Error("Failed to update edit task", "error", err, "task_id", task.ID)
			if attempt == maxRetries {
				return false
			}
			time.Sleep(baseDelay * time.Duration(attempt+1))
			continue
		}

		logger.Info("Successfully generated auto edit caption", "task_id", task.ID, "prompt_length", len(prompt))
		return true
	}

	return false
}

// updateProgress updates the session progress with error handling
func (acm *AutoCaptionManager) updateProgress(session *AutoCaptionSession, status, errorMessage string) {
	session.mutex.Lock()
	session.Progress.Status = status
	session.Progress.ErrorMessage = errorMessage
	if status == "error" || status == "completed" || status == "cancelled" {
		session.Progress.CompletedAt = time.Now().Format(time.RFC3339)
	}
	progress := session.Progress
	session.mutex.Unlock()

	acm.sendProgressUpdate(session.ProjectID, progress)
}

// sendProgressUpdate sends progress updates to connected clients
func (acm *AutoCaptionManager) sendProgressUpdate(projectID string, progress AutoCaptionProgress) {
	acm.progressClientsMu.RLock()
	client, exists := acm.progressClients[projectID]
	acm.progressClientsMu.RUnlock()

	if exists {
		select {
		case client <- progress:
		default:
			// Client channel is full, skip this update
		}
	}
}

// AddProgressClient adds a progress update client for a project
func (acm *AutoCaptionManager) AddProgressClient(projectID string, client chan AutoCaptionProgress) {
	acm.progressClientsMu.Lock()
	acm.progressClients[projectID] = client
	acm.progressClientsMu.Unlock()
}

// RemoveProgressClient removes a progress update client for a project
func (acm *AutoCaptionManager) RemoveProgressClient(projectID string) {
	acm.progressClientsMu.Lock()
	if client, exists := acm.progressClients[projectID]; exists {
		close(client)
		delete(acm.progressClients, projectID)
	}
	acm.progressClientsMu.Unlock()
}