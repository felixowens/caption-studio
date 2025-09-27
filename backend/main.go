package main

import (
	"archive/zip"
	"database/sql"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/corona10/goimagehash"
	"github.com/google/uuid"

	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/webp"
)

var (
	progressClients = make(map[string]chan ProgressUpdate)
	progressMu      sync.RWMutex

	exportProgressClients = make(map[string]chan ExportProgress)
	exportProgressMu      sync.RWMutex
	activeExports         = make(map[string]*ExportStatus)
	activeExportsMu       sync.RWMutex
)

type ProgressUpdate struct {
	ProjectID    string `json:"projectId"`
	Filename     string `json:"filename"`
	Progress     int    `json:"progress"`
	Total        int    `json:"total"`
	Status       string `json:"status"`
	ErrorMessage string `json:"errorMessage,omitempty"`
}

type ExportProgress struct {
	ProjectID    string `json:"projectId"`
	ExportType   string `json:"exportType"`
	Step         string `json:"step"`
	Progress     int    `json:"progress"`
	Total        int    `json:"total"`
	Status       string `json:"status"`
	ErrorMessage string `json:"errorMessage,omitempty"`
	FilePath     string `json:"filePath,omitempty"`
}

type ExportStatus struct {
	ProjectID   string `json:"projectId"`
	ExportType  string `json:"exportType"`
	Status      string `json:"status"`
	Progress    int    `json:"progress"`
	Total       int    `json:"total"`
	FilePath    string `json:"filePath,omitempty"`
	Error       string `json:"error,omitempty"`
	StartTime   string `json:"startTime"`
	CompletedAt string `json:"completedAt,omitempty"`
}

func pingHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "pong")
}

func createProjectHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var project Project
	if err := json.NewDecoder(r.Body).Decode(&project); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	project.ID = uuid.New().String()

	// Set default project type if not provided
	if project.ProjectType == "" {
		project.ProjectType = "edit"
	}

	if err := createProject(&project); err != nil {
		http.Error(w, "Failed to create project", http.StatusInternalServerError)
		logError(r.Context(), "Failed to create project", err, slog.String("project_name", project.Name))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(project)
}

func getProjectHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := r.URL.Path[len("/projects/"):]
	if id == "" {
		http.Error(w, "Project ID is required", http.StatusBadRequest)
		return
	}

	project, err := getProject(id)
	if err != nil {
		http.Error(w, "Failed to get project", http.StatusInternalServerError)
		logError(r.Context(), "Failed to get project", err, slog.String("project_id", id))
		return
	}

	if project == nil {
		http.Error(w, "Project not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(project)
}

func listProjectsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	projects, err := listProjects()
	if err != nil {
		http.Error(w, "Failed to list projects", http.StatusInternalServerError)
		logError(r.Context(), "Failed to list projects", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(projects)
}

func updateProjectHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := r.URL.Path[len("/projects/"):]
	if id == "" {
		http.Error(w, "Project ID is required", http.StatusBadRequest)
		return
	}

	var updatedProject Project
	if err := json.NewDecoder(r.Body).Decode(&updatedProject); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	updatedProject.ID = id // Ensure the ID from the URL is used

	// Check if project exists
	existingProject, err := getProject(id)
	if err != nil {
		http.Error(w, "Failed to get project", http.StatusInternalServerError)
		logError(r.Context(), "Failed to get project for update", err, slog.String("project_id", id))
		return
	}
	if existingProject == nil {
		http.Error(w, "Project not found", http.StatusNotFound)
		return
	}

	if err := updateProject(&updatedProject); err != nil {
		http.Error(w, "Failed to update project", http.StatusInternalServerError)
		logError(r.Context(), "Failed to update project", err, slog.String("project_id", id))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updatedProject)
}

func deleteProjectHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := r.URL.Path[len("/projects/"):]
	if id == "" {
		http.Error(w, "Project ID is required", http.StatusBadRequest)
		return
	}

	// Check if project exists
	existingProject, err := getProject(id)
	if err != nil {
		http.Error(w, "Failed to get project", http.StatusInternalServerError)
		logError(r.Context(), "Failed to get project for deletion", err, slog.String("project_id", id))
		return
	}
	if existingProject == nil {
		http.Error(w, "Project not found", http.StatusNotFound)
		return
	}

	if err := deleteProject(id); err != nil {
		http.Error(w, "Failed to delete project", http.StatusInternalServerError)
		logError(r.Context(), "Failed to delete project", err, slog.String("project_id", id))
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	projectID := r.URL.Query().Get("projectId")
	if projectID == "" {
		http.Error(w, "Project ID is required", http.StatusBadRequest)
		return
	}

	// Check if project exists
	project, err := getProject(projectID)
	if err != nil {
		http.Error(w, "Failed to get project", http.StatusInternalServerError)
		logError(r.Context(), "Failed to get project for upload", err, slog.String("project_id", projectID))
		return
	}
	if project == nil {
		http.Error(w, "Project not found", http.StatusNotFound)
		return
	}

	// Parse multipart form
	err = r.ParseMultipartForm(32 << 20) // 32MB max memory
	if err != nil {
		http.Error(w, "Error parsing multipart form", http.StatusBadRequest)
		return
	}

	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		http.Error(w, "No files provided", http.StatusBadRequest)
		return
	}

	// Create project directory
	projectDir := filepath.Join("data", "projects", projectID, "images")
	err = os.MkdirAll(projectDir, 0755)
	if err != nil {
		http.Error(w, "Error creating project directory", http.StatusInternalServerError)
		return
	}

	// Process files asynchronously
	logInfo(r.Context(), "Upload started",
		slog.String("project_id", projectID),
		slog.Int("file_count", len(files)),
	)
	go processUploadedFiles(projectID, files, projectDir)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Upload started",
		"count":   len(files),
	})
}

func processUploadedFiles(projectID string, files []*multipart.FileHeader, projectDir string) {
	total := len(files)
	processedImages := make([]Image, 0, total)

	for i, fileHeader := range files {
		// Send progress update
		sendProgressUpdate(projectID, ProgressUpdate{
			ProjectID: projectID,
			Filename:  fileHeader.Filename,
			Progress:  i + 1,
			Total:     total,
			Status:    "processing",
		})

		// Check if file already exists by path
		imagePath := filepath.Join("images", fileHeader.Filename)
		exists, err := imageExistsByPath(projectID, imagePath)
		if err != nil {
			sendProgressUpdate(projectID, ProgressUpdate{
				ProjectID:    projectID,
				Filename:     fileHeader.Filename,
				Progress:     i + 1,
				Total:        total,
				Status:       "error",
				ErrorMessage: fmt.Sprintf("Error checking existing file: %v", err),
			})
			continue
		}

		if exists {
			logger.Info("Skipping duplicate file",
				"project_id", projectID,
				"filename", fileHeader.Filename,
			)
			sendProgressUpdate(projectID, ProgressUpdate{
				ProjectID:    projectID,
				Filename:     fileHeader.Filename,
				Progress:     i + 1,
				Total:        total,
				Status:       "skipped",
				ErrorMessage: "File already exists",
			})
			continue
		}

		// Open uploaded file
		file, err := fileHeader.Open()
		if err != nil {
			sendProgressUpdate(projectID, ProgressUpdate{
				ProjectID:    projectID,
				Filename:     fileHeader.Filename,
				Progress:     i + 1,
				Total:        total,
				Status:       "error",
				ErrorMessage: fmt.Sprintf("Error opening file: %v", err),
			})
			continue
		}

		// Read file content
		content, err := io.ReadAll(file)
		file.Close()
		if err != nil {
			sendProgressUpdate(projectID, ProgressUpdate{
				ProjectID:    projectID,
				Filename:     fileHeader.Filename,
				Progress:     i + 1,
				Total:        total,
				Status:       "error",
				ErrorMessage: fmt.Sprintf("Error reading file: %v", err),
			})
			continue
		}

		// Validate image
		reader := strings.NewReader(string(content))
		img, _, err := image.Decode(reader)
		if err != nil {
			logger.Error("Invalid image format",
				"error", err,
				"project_id", projectID,
				"filename", fileHeader.Filename,
			)
			sendProgressUpdate(projectID, ProgressUpdate{
				ProjectID:    projectID,
				Filename:     fileHeader.Filename,
				Progress:     i + 1,
				Total:        total,
				Status:       "error",
				ErrorMessage: fmt.Sprintf("Invalid image format: %v", err),
			})
			continue
		}

		// Compute pHash
		hash, err := goimagehash.PerceptionHash(img)
		if err != nil {
			sendProgressUpdate(projectID, ProgressUpdate{
				ProjectID:    projectID,
				Filename:     fileHeader.Filename,
				Progress:     i + 1,
				Total:        total,
				Status:       "error",
				ErrorMessage: fmt.Sprintf("Error computing hash: %v", err),
			})
			continue
		}

		// Check if similar image exists by hash
		hashExists, err := imageExistsByHash(projectID, hash.ToString(), 0)
		if err != nil {
			logger.Warn("Error checking hash duplicates",
				"error", err,
				"project_id", projectID,
				"filename", fileHeader.Filename,
			)
		} else if hashExists {
			logger.Info("Skipping duplicate image by hash",
				"project_id", projectID,
				"filename", fileHeader.Filename,
			)
			sendProgressUpdate(projectID, ProgressUpdate{
				ProjectID:    projectID,
				Filename:     fileHeader.Filename,
				Progress:     i + 1,
				Total:        total,
				Status:       "skipped",
				ErrorMessage: "Similar image already exists",
			})
			continue
		}

		// Save file to disk
		filename := fileHeader.Filename
		filePath := filepath.Join(projectDir, filename)
		destFile, err := os.Create(filePath)
		if err != nil {
			sendProgressUpdate(projectID, ProgressUpdate{
				ProjectID:    projectID,
				Filename:     fileHeader.Filename,
				Progress:     i + 1,
				Total:        total,
				Status:       "error",
				ErrorMessage: fmt.Sprintf("Error creating file: %v", err),
			})
			continue
		}

		_, err = destFile.Write(content)
		destFile.Close()
		if err != nil {
			sendProgressUpdate(projectID, ProgressUpdate{
				ProjectID:    projectID,
				Filename:     fileHeader.Filename,
				Progress:     i + 1,
				Total:        total,
				Status:       "error",
				ErrorMessage: fmt.Sprintf("Error writing file: %v", err),
			})
			continue
		}

		// Create image record
		imageRecord := Image{
			ID:        uuid.New().String(),
			ProjectID: projectID,
			Path:      imagePath,
			PHash:     hash.ToString(),
		}

		processedImages = append(processedImages, imageRecord)
	}

	// Store images in database
	if len(processedImages) > 0 {
		if err := createImages(processedImages); err != nil {
			logger.Error("Error storing images in database",
				"error", err,
				"project_id", projectID,
				"image_count", len(processedImages),
			)
			sendProgressUpdate(projectID, ProgressUpdate{
				ProjectID:    projectID,
				Progress:     total,
				Total:        total,
				Status:       "error",
				ErrorMessage: "Failed to store images in database",
			})
			return
		}
		logger.Info("Images stored successfully",
			"project_id", projectID,
			"image_count", len(processedImages),
		)
	}

	// Send completion update
	sendProgressUpdate(projectID, ProgressUpdate{
		ProjectID: projectID,
		Progress:  total,
		Total:     total,
		Status:    "completed",
	})
}

func sendProgressUpdate(projectID string, update ProgressUpdate) {
	progressMu.RLock()
	client, exists := progressClients[projectID]
	progressMu.RUnlock()

	if exists {
		select {
		case client <- update:
		default:
			// Client channel is full, skip this update
		}
	}
}

func sendExportProgress(projectID string, progress ExportProgress) {
	exportProgressMu.RLock()
	client, exists := exportProgressClients[projectID]
	exportProgressMu.RUnlock()

	if exists {
		select {
		case client <- progress:
		default:
			// Client channel is full, skip this update
		}
	}
}

func updateExportStatus(projectID string, status *ExportStatus) {
	activeExportsMu.Lock()
	activeExports[projectID] = status
	activeExportsMu.Unlock()
}

func getExportStatus(projectID string) *ExportStatus {
	activeExportsMu.RLock()
	defer activeExportsMu.RUnlock()
	if status, exists := activeExports[projectID]; exists {
		return status
	}
	return nil
}

func progressHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	projectID := r.URL.Query().Get("projectId")
	if projectID == "" {
		http.Error(w, "Project ID is required", http.StatusBadRequest)
		return
	}

	// Set headers for SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Create progress channel for this client
	progressCh := make(chan ProgressUpdate, 100)
	progressMu.Lock()
	progressClients[projectID] = progressCh
	progressMu.Unlock()

	// Clean up when client disconnects
	defer func() {
		progressMu.Lock()
		delete(progressClients, projectID)
		progressMu.Unlock()
		close(progressCh)
	}()

	// Send events to client
	for {
		select {
		case update := <-progressCh:
			data, _ := json.Marshal(update)
			fmt.Fprintf(w, "data: %s\n\n", data)
			w.(http.Flusher).Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func getImagesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	projectID := r.URL.Query().Get("projectId")
	if projectID == "" {
		http.Error(w, "Project ID is required", http.StatusBadRequest)
		return
	}

	projectImages, err := getImagesByProjectID(projectID)
	if err != nil {
		http.Error(w, "Failed to get images", http.StatusInternalServerError)
		logError(r.Context(), "Failed to get images", err, slog.String("project_id", projectID))
		return
	}

	if projectImages == nil {
		projectImages = []Image{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(projectImages)
}

func deleteImageHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract project ID and image ID from URL
	// URL format: /projects/{projectId}/images/{imageId}
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/projects/"), "/")
	if len(pathParts) < 3 || pathParts[1] != "images" {
		http.Error(w, "Invalid image path", http.StatusBadRequest)
		return
	}

	projectID := pathParts[0]
	imageID := pathParts[2]

	// Check if project exists
	project, err := getProject(projectID)
	if err != nil {
		http.Error(w, "Failed to get project", http.StatusInternalServerError)
		logError(r.Context(), "Failed to get project for image deletion", err, slog.String("project_id", projectID))
		return
	}
	if project == nil {
		http.Error(w, "Project not found", http.StatusNotFound)
		return
	}

	// Get image details before deletion
	image, err := getImage(imageID)
	if err != nil {
		http.Error(w, "Failed to get image", http.StatusInternalServerError)
		logError(r.Context(), "Failed to get image for deletion", err, slog.String("image_id", imageID))
		return
	}
	if image == nil {
		http.Error(w, "Image not found", http.StatusNotFound)
		return
	}

	// Verify image belongs to the project
	if image.ProjectID != projectID {
		http.Error(w, "Image does not belong to this project", http.StatusBadRequest)
		return
	}

	// Delete image file from disk
	filePath := filepath.Join("data", "projects", projectID, image.Path)
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		logError(r.Context(), "Failed to delete image file", err,
			slog.String("file_path", filePath),
			slog.String("image_id", imageID))
	}

	// Delete image from database (this will cascade delete related tasks)
	if err := deleteImage(imageID); err != nil {
		http.Error(w, "Failed to delete image", http.StatusInternalServerError)
		logError(r.Context(), "Failed to delete image from database", err, slog.String("image_id", imageID))
		return
	}

	logInfo(r.Context(), "Image deleted successfully",
		slog.String("project_id", projectID),
		slog.String("image_id", imageID),
		slog.String("path", image.Path))

	w.WriteHeader(http.StatusNoContent)
}

type SimilarImage struct {
	Image    Image
	Distance int
}

type TaskGenerationRequest struct {
	SimilarityThreshold int `json:"similarityThreshold"`
	MaxCandidates       int `json:"maxCandidates"`
}

type TaskGenerationResponse struct {
	TasksCreated      int     `json:"tasksCreated"`
	AverageCandidates float64 `json:"averageCandidates"`
}

func parseImageHash(hashString string) (*goimagehash.ImageHash, error) {
	return goimagehash.ImageHashFromString(hashString)
}

func findSimilarImages(targetImage Image, allImages []Image, threshold int) ([]SimilarImage, error) {
	targetHash, err := parseImageHash(targetImage.PHash)
	if err != nil {
		return nil, fmt.Errorf("failed to parse target hash: %v", err)
	}

	var similar []SimilarImage
	for _, img := range allImages {
		if img.ID == targetImage.ID {
			continue
		}

		imgHash, err := parseImageHash(img.PHash)
		if err != nil {
			logger.Warn("Failed to parse image hash",
				"error", err,
				"image_id", img.ID,
			)
			continue
		}

		distance, err := targetHash.Distance(imgHash)
		if err != nil {
			logger.Warn("Failed to calculate image distance",
				"error", err,
				"image_id", img.ID,
			)
			continue
		}

		logger.Debug("Image distance calculated",
			"image_id", img.ID,
			"distance", distance,
		)

		if distance <= threshold {
			similar = append(similar, SimilarImage{
				Image:    img,
				Distance: distance,
			})
		}
	}

	// Sort by distance (most similar first)
	for i := 0; i < len(similar)-1; i++ {
		for j := i + 1; j < len(similar); j++ {
			if similar[i].Distance > similar[j].Distance {
				similar[i], similar[j] = similar[j], similar[i]
			}
		}
	}

	return similar, nil
}

func generateCaptionTasksForProject(projectID string) (*TaskGenerationResponse, error) {
	images, err := getImagesByProjectID(projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get images: %v", err)
	}

	if len(images) == 0 {
		return &TaskGenerationResponse{TasksCreated: 0, AverageCandidates: 0}, nil
	}

	var tasksCreated int
	for _, img := range images {
		// Check if caption task already exists for this image
		exists, err := captionTaskExistsForImage(projectID, img.ID)
		if err != nil {
			logger.Warn("Error checking if caption task exists",
				"error", err,
				"image_id", img.ID,
			)
			continue
		}
		if exists {
			logger.Debug("Caption task already exists for image, skipping",
				"image_id", img.ID,
				"project_id", projectID,
			)
			continue
		}

		// Create caption task
		task := &CaptionTask{
			ID:        uuid.New().String(),
			ProjectID: projectID,
			ImageID:   img.ID,
			Caption:   sql.NullString{}, // Will be set during annotation
			Status:    "pending",
			Skipped:   false,
		}

		logger.Debug("Creating caption task",
			"task_id", task.ID,
			"project_id", projectID,
			"image_id", img.ID,
		)
		if err := createCaptionTask(task); err != nil {
			logger.Error("Error creating caption task",
				"error", err,
				"task_id", task.ID,
				"project_id", projectID,
				"image_id", img.ID,
			)
			continue
		}

		tasksCreated++
	}

	return &TaskGenerationResponse{
		TasksCreated:      tasksCreated,
		AverageCandidates: 1, // Each caption task has one image
	}, nil
}

func generateTasksForProject(projectID string, threshold, maxCandidates int) (*TaskGenerationResponse, error) {
	images, err := getImagesByProjectID(projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get images: %v", err)
	}

	if len(images) == 0 {
		return &TaskGenerationResponse{TasksCreated: 0, AverageCandidates: 0}, nil
	}

	var totalCandidates int
	var tasksCreated int
	for _, img := range images {
		// Check if task already exists for this image
		exists, err := taskExistsForImageA(projectID, img.ID)
		if err != nil {
			logger.Warn("Error checking if task exists",
				"error", err,
				"image_id", img.ID,
			)
			continue
		}
		if exists {
			logger.Debug("Task already exists for image, skipping",
				"image_id", img.ID,
				"project_id", projectID,
			)
			continue
		}

		similarImages, err := findSimilarImages(img, images, threshold)
		if err != nil {
			logger.Warn("Error finding similar images",
				"error", err,
				"image_id", img.ID,
			)
			continue
		}

		// Limit candidates
		candidates := similarImages
		if len(candidates) > maxCandidates {
			candidates = candidates[:maxCandidates]
		}

		// Extract candidate IDs
		var candidateIDs []string
		for _, candidate := range candidates {
			candidateIDs = append(candidateIDs, candidate.Image.ID)
		}

		// Auto-select the best match (lowest distance = highest similarity) as Image B
		var selectedImageB sql.NullString
		if len(candidates) > 0 {
			bestMatch := candidates[0] // candidates are already sorted by distance (best first)
			selectedImageB = sql.NullString{
				String: bestMatch.Image.ID,
				Valid:  true,
			}
		}

		// Create task
		task := &Task{
			ID:            uuid.New().String(),
			ProjectID:     projectID,
			ImageAID:      img.ID,
			ImageBId:      selectedImageB,
			Prompt:        sql.NullString{}, // Will be set during annotation
			Status:        "pending",
			Skipped:       false,
			CandidateBIds: candidateIDs,
		}

		logger.Debug("Creating task with auto-selected Image B",
			"task_id", task.ID,
			"project_id", projectID,
			"image_a_id", img.ID,
			"image_b_id", selectedImageB.String,
			"auto_selected", selectedImageB.Valid,
			"candidate_count", len(candidateIDs),
		)
		if err := createTask(task); err != nil {
			logger.Error("Error creating task",
				"error", err,
				"task_id", task.ID,
				"project_id", projectID,
				"image_id", img.ID,
			)
			continue
		}

		tasksCreated++
		totalCandidates += len(candidateIDs)
	}

	var averageCandidates float64
	if tasksCreated > 0 {
		averageCandidates = float64(totalCandidates) / float64(tasksCreated)
	}
	return &TaskGenerationResponse{
		TasksCreated:      tasksCreated,
		AverageCandidates: averageCandidates,
	}, nil
}

func generateTasksHandler(w http.ResponseWriter, r *http.Request) {
	projectID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/projects/"), "/generate-tasks")
	if projectID == "" {
		http.Error(w, "Project ID is required", http.StatusBadRequest)
		return
	}

	// Check if project exists
	project, err := getProject(projectID)
	if err != nil {
		http.Error(w, "Failed to get project", http.StatusInternalServerError)
		logError(r.Context(), "Failed to get project for task generation", err, slog.String("project_id", projectID))
		return
	}
	if project == nil {
		http.Error(w, "Project not found", http.StatusNotFound)
		return
	}

	// Parse request body
	var req TaskGenerationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Use defaults if parsing fails
		req.SimilarityThreshold = 10
		req.MaxCandidates = 5
	}

	// Validate parameters
	if req.SimilarityThreshold <= 0 {
		req.SimilarityThreshold = 10
	}
	if req.MaxCandidates <= 0 {
		req.MaxCandidates = 5
	}

	// Generate tasks based on project type
	var response *TaskGenerationResponse

	if project.ProjectType == "caption" {
		logInfo(r.Context(), "Generating caption tasks",
			slog.String("project_id", projectID),
			slog.String("project_type", project.ProjectType),
		)
		response, err = generateCaptionTasksForProject(projectID)
	} else {
		logInfo(r.Context(), "Generating edit tasks",
			slog.String("project_id", projectID),
			slog.String("project_type", project.ProjectType),
			slog.Int("similarity_threshold", req.SimilarityThreshold),
			slog.Int("max_candidates", req.MaxCandidates),
		)
		response, err = generateTasksForProject(projectID, req.SimilarityThreshold, req.MaxCandidates)
	}

	if err != nil {
		http.Error(w, "Failed to generate tasks", http.StatusInternalServerError)
		logError(r.Context(), "Failed to generate tasks", err, slog.String("project_id", projectID))
		return
	}
	logInfo(r.Context(), "Tasks generated successfully",
		slog.String("project_id", projectID),
		slog.String("project_type", project.ProjectType),
		slog.Int("tasks_created", response.TasksCreated),
		slog.Float64("average_candidates", response.AverageCandidates),
	)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func getTasksHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	projectID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/projects/"), "/tasks")
	if projectID == "" {
		http.Error(w, "Project ID is required", http.StatusBadRequest)
		return
	}

	// Check if project exists
	project, err := getProject(projectID)
	if err != nil {
		http.Error(w, "Failed to get project", http.StatusInternalServerError)
		logError(r.Context(), "Failed to get project for tasks", err, slog.String("project_id", projectID))
		return
	}
	if project == nil {
		http.Error(w, "Project not found", http.StatusNotFound)
		return
	}

	tasks, err := getTasksByProjectID(projectID)
	if err != nil {
		http.Error(w, "Failed to get tasks", http.StatusInternalServerError)
		logError(r.Context(), "Failed to get tasks", err, slog.String("project_id", projectID))
		return
	}

	if tasks == nil {
		tasks = []Task{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tasks)
}

func getTaskHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	taskID := r.URL.Path[len("/tasks/"):]
	if taskID == "" {
		http.Error(w, "Task ID is required", http.StatusBadRequest)
		return
	}

	task, err := getTask(taskID)
	if err != nil {
		http.Error(w, "Failed to get task", http.StatusInternalServerError)
		logError(r.Context(), "Failed to get task", err, slog.String("task_id", taskID))
		return
	}

	if task == nil {
		http.Error(w, "Task not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(task)
}

func getCaptionTasksHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	projectID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/projects/"), "/caption-tasks")
	if projectID == "" {
		http.Error(w, "Project ID is required", http.StatusBadRequest)
		return
	}

	// Check if project exists and is caption type
	project, err := getProject(projectID)
	if err != nil {
		http.Error(w, "Failed to get project", http.StatusInternalServerError)
		logError(r.Context(), "Failed to get project for caption tasks", err, slog.String("project_id", projectID))
		return
	}
	if project == nil {
		http.Error(w, "Project not found", http.StatusNotFound)
		return
	}

	tasks, err := getCaptionTasksByProjectID(projectID)
	if err != nil {
		http.Error(w, "Failed to get caption tasks", http.StatusInternalServerError)
		logError(r.Context(), "Failed to get caption tasks", err, slog.String("project_id", projectID))
		return
	}

	if tasks == nil {
		tasks = []CaptionTask{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tasks)
}

func getCaptionTaskHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	taskID := r.URL.Path[len("/caption-tasks/"):]
	if taskID == "" {
		http.Error(w, "Caption task ID is required", http.StatusBadRequest)
		return
	}

	task, err := getCaptionTask(taskID)
	if err != nil {
		http.Error(w, "Failed to get caption task", http.StatusInternalServerError)
		logError(r.Context(), "Failed to get caption task", err, slog.String("task_id", taskID))
		return
	}

	if task == nil {
		http.Error(w, "Caption task not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(task)
}

func updateCaptionTaskHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "PUT" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	taskID := r.URL.Path[len("/caption-tasks/"):]
	if taskID == "" {
		http.Error(w, "Caption task ID is required", http.StatusBadRequest)
		return
	}

	// Check if task exists
	existingTask, err := getCaptionTask(taskID)
	if err != nil {
		http.Error(w, "Failed to get caption task", http.StatusInternalServerError)
		logError(r.Context(), "Failed to get caption task for update", err, slog.String("task_id", taskID))
		return
	}
	if existingTask == nil {
		http.Error(w, "Caption task not found", http.StatusNotFound)
		return
	}

	var updatedTask CaptionTask
	if err := json.NewDecoder(r.Body).Decode(&updatedTask); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	updatedTask.ID = taskID // Ensure the ID from the URL is used

	if err := updateCaptionTask(&updatedTask); err != nil {
		http.Error(w, "Failed to update caption task", http.StatusInternalServerError)
		logError(r.Context(), "Failed to update caption task", err, slog.String("task_id", taskID))
		return
	}

	// Return the updated task
	task, err := getCaptionTask(taskID)
	if err != nil {
		http.Error(w, "Failed to get updated caption task", http.StatusInternalServerError)
		logError(r.Context(), "Failed to get updated caption task", err, slog.String("task_id", taskID))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(task)
}

func updateTaskHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "PUT" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	taskID := r.URL.Path[len("/tasks/"):]
	if taskID == "" {
		http.Error(w, "Task ID is required", http.StatusBadRequest)
		return
	}

	// Check if task exists
	existingTask, err := getTask(taskID)
	if err != nil {
		http.Error(w, "Failed to get task", http.StatusInternalServerError)
		logError(r.Context(), "Failed to get task for update", err, slog.String("task_id", taskID))
		return
	}
	if existingTask == nil {
		http.Error(w, "Task not found", http.StatusNotFound)
		return
	}

	var updatedTask Task
	if err := json.NewDecoder(r.Body).Decode(&updatedTask); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	updatedTask.ID = taskID // Ensure the ID from the URL is used

	if err := updateTask(&updatedTask); err != nil {
		http.Error(w, "Failed to update task", http.StatusInternalServerError)
		logError(r.Context(), "Failed to update task", err, slog.String("task_id", taskID))
		return
	}

	// Return the updated task
	task, err := getTask(taskID)
	if err != nil {
		http.Error(w, "Failed to get updated task", http.StatusInternalServerError)
		logError(r.Context(), "Failed to get updated task", err, slog.String("task_id", taskID))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(task)
}

func serveImageHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract project ID and image path from URL
	// URL format: /projects/{projectId}/images/{imagePath}
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/projects/"), "/")
	if len(pathParts) < 3 || pathParts[1] != "images" {
		http.Error(w, "Invalid image path", http.StatusBadRequest)
		return
	}

	projectID := pathParts[0]
	imagePath := strings.Join(pathParts[2:], "/")

	// Construct file path
	filePath := filepath.Join("data", "projects", projectID, "images", imagePath)

	// Security check: ensure the path is within the project directory
	absProjectDir, err := filepath.Abs(filepath.Join("data", "projects", projectID))
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	absFilePath, err := filepath.Abs(filePath)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if !strings.HasPrefix(absFilePath, absProjectDir) {
		http.Error(w, "Access denied", http.StatusForbidden)
		return
	}

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		http.Error(w, "Image not found", http.StatusNotFound)
		return
	}

	// Serve the file
	http.ServeFile(w, r, filePath)
}

func exportJSONLHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	projectID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/projects/"), "/export/jsonl")
	if projectID == "" {
		http.Error(w, "Project ID is required", http.StatusBadRequest)
		return
	}

	// Check if project exists
	project, err := getProject(projectID)
	if err != nil {
		http.Error(w, "Failed to get project", http.StatusInternalServerError)
		logError(r.Context(), "Failed to get project for JSONL export", err, slog.String("project_id", projectID))
		return
	}
	if project == nil {
		http.Error(w, "Project not found", http.StatusNotFound)
		return
	}

	// Handle different project types for export
	if project.ProjectType == "caption" {
		// Caption project export
		captionTasks, err := getCaptionTasksByProjectID(projectID)
		if err != nil {
			http.Error(w, "Failed to get caption tasks", http.StatusInternalServerError)
			logError(r.Context(), "Failed to get caption tasks for JSONL export", err, slog.String("project_id", projectID))
			return
		}

		// Get all images for path lookup
		images, err := getImagesByProjectID(projectID)
		if err != nil {
			http.Error(w, "Failed to get images", http.StatusInternalServerError)
			logError(r.Context(), "Failed to get images for JSONL export", err, slog.String("project_id", projectID))
			return
		}

		// Create image lookup map
		imageMap := make(map[string]*Image)
		for i := range images {
			imageMap[images[i].ID] = &images[i]
		}

		// Set response headers for file download
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s_captions.jsonl\"", project.Name))

		// Write JSONL format for captions
		for _, task := range captionTasks {
			// Only export completed tasks (not skipped, has caption)
			if task.Skipped || !task.Caption.Valid {
				continue
			}

			image := imageMap[task.ImageID]
			if image == nil {
				continue
			}

			// Create export record
			record := map[string]interface{}{
				"image":   image.Path,
				"caption": task.Caption.String,
			}

			// Write JSON line
			jsonData, err := json.Marshal(record)
			if err != nil {
				logError(r.Context(), "Failed to marshal caption task record", err, slog.String("task_id", task.ID))
				continue
			}

			w.Write(jsonData)
			w.Write([]byte("\n"))
		}
	} else {
		// Edit project export (existing functionality)
		tasks, err := getTasksByProjectID(projectID)
		if err != nil {
			http.Error(w, "Failed to get tasks", http.StatusInternalServerError)
			logError(r.Context(), "Failed to get tasks for JSONL export", err, slog.String("project_id", projectID))
			return
		}

		// Get all images for path lookup
		images, err := getImagesByProjectID(projectID)
		if err != nil {
			http.Error(w, "Failed to get images", http.StatusInternalServerError)
			logError(r.Context(), "Failed to get images for JSONL export", err, slog.String("project_id", projectID))
			return
		}

		// Create image lookup map
		imageMap := make(map[string]*Image)
		for i := range images {
			imageMap[images[i].ID] = &images[i]
		}

		// Set response headers for file download
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s_annotations.jsonl\"", project.Name))

		// Write JSONL format for edit tasks
		for _, task := range tasks {
			// Only export completed tasks (not skipped, has imageB or prompt)
			if task.Skipped || (!task.ImageBId.Valid && !task.Prompt.Valid) {
				continue
			}

			imageA := imageMap[task.ImageAID]
			if imageA == nil {
				continue
			}

			// Create export record
			record := map[string]interface{}{
				"a": imageA.Path,
			}

			if task.ImageBId.Valid {
				imageB := imageMap[task.ImageBId.String]
				if imageB != nil {
					record["b"] = imageB.Path
				}
			}

			if task.Prompt.Valid {
				record["prompt"] = task.Prompt.String
			}

			// Write JSON line
			jsonData, err := json.Marshal(record)
			if err != nil {
				logError(r.Context(), "Failed to marshal task record", err, slog.String("task_id", task.ID))
				continue
			}

			w.Write(jsonData)
			w.Write([]byte("\n"))
		}
	}

	logInfo(r.Context(), "JSONL export completed", slog.String("project_id", projectID))
}

func exportAIToolkitHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	projectID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/projects/"), "/export/ai-toolkit")
	if projectID == "" {
		http.Error(w, "Project ID is required", http.StatusBadRequest)
		return
	}

	// Check if there's already an active export
	if status := getExportStatus(projectID); status != nil && status.Status == "processing" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message": "Export already in progress",
			"status":  status,
		})
		return
	}

	// Check if project exists
	project, err := getProject(projectID)
	if err != nil {
		http.Error(w, "Failed to get project", http.StatusInternalServerError)
		logError(r.Context(), "Failed to get project for AI-toolkit export", err, slog.String("project_id", projectID))
		return
	}
	if project == nil {
		http.Error(w, "Project not found", http.StatusNotFound)
		return
	}

	// AI-toolkit export is only available for edit projects
	if project.ProjectType == "caption" {
		http.Error(w, "AI-toolkit export is not available for caption projects", http.StatusBadRequest)
		return
	}

	// Start async export
	go asyncExportAIToolkit(projectID, project)

	// Return immediate response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":   "Export started",
		"projectId": projectID,
		"type":      "ai-toolkit",
	})
}

func asyncExportAIToolkit(projectID string, project *Project) {
	startTime := "2023-01-01T00:00:00Z" // You might want to use actual timestamp

	// Initialize export status
	status := &ExportStatus{
		ProjectID:  projectID,
		ExportType: "ai-toolkit",
		Status:     "processing",
		StartTime:  startTime,
	}
	updateExportStatus(projectID, status)

	// Send initial progress
	sendExportProgress(projectID, ExportProgress{
		ProjectID:  projectID,
		ExportType: "ai-toolkit",
		Step:       "initializing",
		Status:     "processing",
	})

	// Get completed tasks
	tasks, err := getTasksByProjectID(projectID)
	if err != nil {
		status.Status = "error"
		status.Error = err.Error()
		updateExportStatus(projectID, status)
		sendExportProgress(projectID, ExportProgress{
			ProjectID:    projectID,
			ExportType:   "ai-toolkit",
			Status:       "error",
			ErrorMessage: err.Error(),
		})
		return
	}

	// Get all images for path lookup
	images, err := getImagesByProjectID(projectID)
	if err != nil {
		status.Status = "error"
		status.Error = err.Error()
		updateExportStatus(projectID, status)
		sendExportProgress(projectID, ExportProgress{
			ProjectID:    projectID,
			ExportType:   "ai-toolkit",
			Status:       "error",
			ErrorMessage: err.Error(),
		})
		return
	}

	// Create image lookup map
	imageMap := make(map[string]*Image)
	for i := range images {
		imageMap[images[i].ID] = &images[i]
	}

	// Count valid tasks for progress tracking
	validTasks := 0
	for _, task := range tasks {
		if !task.Skipped && task.ImageBId.Valid && task.Prompt.Valid {
			imageA := imageMap[task.ImageAID]
			imageB := imageMap[task.ImageBId.String]
			if imageA != nil && imageB != nil {
				validTasks++
			}
		}
	}

	status.Total = validTasks
	updateExportStatus(projectID, status)

	// Create temporary export directory
	exportDir := filepath.Join("data", "exports", projectID+"-ai-toolkit")
	sourceDir := filepath.Join(exportDir, "source")
	targetDir := filepath.Join(exportDir, "target")

	// Clean and create directories
	os.RemoveAll(exportDir)
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		status.Status = "error"
		status.Error = err.Error()
		updateExportStatus(projectID, status)
		return
	}
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		status.Status = "error"
		status.Error = err.Error()
		updateExportStatus(projectID, status)
		return
	}

	sendExportProgress(projectID, ExportProgress{
		ProjectID:  projectID,
		ExportType: "ai-toolkit",
		Step:       "copying_files",
		Status:     "processing",
		Total:      validTasks,
	})

	// Process completed tasks with worker pool
	exportCount := 0
	processedTasks := 0
	var exportCountMu sync.Mutex

	// Worker pool for file operations
	const numWorkers = 4
	taskChan := make(chan Task, validTasks)
	resultChan := make(chan bool, validTasks)

	// Start workers
	for i := 0; i < numWorkers; i++ {
		go func() {
			for task := range taskChan {
				exportCountMu.Lock()
				currentCount := exportCount
				exportCount++
				exportCountMu.Unlock()

				success := processTaskForAIToolkit(task, imageMap, projectID, sourceDir, targetDir, currentCount)
				resultChan <- success
			}
		}()
	}

	// Send tasks to workers
	for _, task := range tasks {
		if !task.Skipped && task.ImageBId.Valid && task.Prompt.Valid {
			imageA := imageMap[task.ImageAID]
			imageB := imageMap[task.ImageBId.String]
			if imageA != nil && imageB != nil {
				taskChan <- task
			}
		}
	}
	close(taskChan)

	// Collect results and update progress
	for i := 0; i < validTasks; i++ {
		<-resultChan
		processedTasks++
		status.Progress = processedTasks
		updateExportStatus(projectID, status)

		sendExportProgress(projectID, ExportProgress{
			ProjectID:  projectID,
			ExportType: "ai-toolkit",
			Step:       "copying_files",
			Progress:   processedTasks,
			Total:      validTasks,
			Status:     "processing",
		})
	}

	sendExportProgress(projectID, ExportProgress{
		ProjectID:  projectID,
		ExportType: "ai-toolkit",
		Step:       "creating_zip",
		Status:     "processing",
	})

	// Create ZIP archive with progress
	zipPath := filepath.Join("data", "exports", project.Name+"_ai-toolkit.zip")
	if err := createZipArchiveWithProgress(exportDir, zipPath, projectID); err != nil {
		status.Status = "error"
		status.Error = err.Error()
		updateExportStatus(projectID, status)
		sendExportProgress(projectID, ExportProgress{
			ProjectID:    projectID,
			ExportType:   "ai-toolkit",
			Status:       "error",
			ErrorMessage: err.Error(),
		})
		return
	}

	// Update final status
	status.Status = "completed"
	status.FilePath = zipPath
	status.CompletedAt = "2023-01-01T00:05:00Z" // You might want to use actual timestamp
	updateExportStatus(projectID, status)

	sendExportProgress(projectID, ExportProgress{
		ProjectID:  projectID,
		ExportType: "ai-toolkit",
		Status:     "completed",
		FilePath:   zipPath,
	})

	// Clean up temporary directory (but keep zip file for download)
	go func() {
		os.RemoveAll(exportDir)
	}()

	exportCountMu.Lock()
	finalExportCount := exportCount
	exportCountMu.Unlock()

	logger.Info("AI-toolkit export completed",
		"project_id", projectID,
		"exported_pairs", finalExportCount)
}

func processTaskForAIToolkit(task Task, imageMap map[string]*Image, projectID, sourceDir, targetDir string, exportCount int) bool {
	imageA := imageMap[task.ImageAID]
	imageB := imageMap[task.ImageBId.String]
	if imageA == nil || imageB == nil {
		return false
	}

	// Generate unique filename for this pair
	baseName := fmt.Sprintf("pair_%04d", exportCount+1)

	// Copy source image
	sourceImagePath := filepath.Join("data", "projects", projectID, imageA.Path)
	destSourcePath := filepath.Join(sourceDir, baseName+filepath.Ext(imageA.Path))
	if err := copyFile(sourceImagePath, destSourcePath); err != nil {
		logger.Error("Failed to copy source image", "error", err)
		return false
	}

	// Copy target image
	targetImagePath := filepath.Join("data", "projects", projectID, imageB.Path)
	destTargetPath := filepath.Join(targetDir, baseName+filepath.Ext(imageB.Path))
	if err := copyFile(targetImagePath, destTargetPath); err != nil {
		logger.Error("Failed to copy target image", "error", err)
		return false
	}

	// Write caption files in both source and target folders
	sourceCaptionPath := filepath.Join(sourceDir, baseName+".txt")
	targetCaptionPath := filepath.Join(targetDir, baseName+".txt")

	captionContent := []byte(task.Prompt.String)

	if err := os.WriteFile(sourceCaptionPath, captionContent, 0644); err != nil {
		logger.Error("Failed to write source caption file", "error", err)
		return false
	}

	if err := os.WriteFile(targetCaptionPath, captionContent, 0644); err != nil {
		logger.Error("Failed to write target caption file", "error", err)
		return false
	}

	return true
}

func exportImageTextPairsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	projectID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/projects/"), "/export/image-text-pairs")
	if projectID == "" {
		http.Error(w, "Project ID is required", http.StatusBadRequest)
		return
	}

	// Check if there's already an active export
	if status := getExportStatus(projectID); status != nil && status.Status == "processing" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message": "Export already in progress",
			"status":  status,
		})
		return
	}

	// Check if project exists
	project, err := getProject(projectID)
	if err != nil {
		http.Error(w, "Failed to get project", http.StatusInternalServerError)
		logError(r.Context(), "Failed to get project for image-text-pairs export", err, slog.String("project_id", projectID))
		return
	}
	if project == nil {
		http.Error(w, "Project not found", http.StatusNotFound)
		return
	}

	// Image-text-pairs export is only available for caption projects
	if project.ProjectType != "caption" {
		http.Error(w, "Image-text-pairs export is only available for caption projects", http.StatusBadRequest)
		return
	}

	// Start async export
	go asyncExportImageTextPairs(projectID, project)

	// Return immediate response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":   "Export started",
		"projectId": projectID,
		"type":      "image-text-pairs",
	})
}

func asyncExportImageTextPairs(projectID string, project *Project) {
	startTime := "2023-01-01T00:00:00Z" // You might want to use actual timestamp

	// Initialize export status
	status := &ExportStatus{
		ProjectID:  projectID,
		ExportType: "image-text-pairs",
		Status:     "processing",
		StartTime:  startTime,
	}
	updateExportStatus(projectID, status)

	// Send initial progress
	sendExportProgress(projectID, ExportProgress{
		ProjectID:  projectID,
		ExportType: "image-text-pairs",
		Step:       "initializing",
		Status:     "processing",
	})

	// Get completed caption tasks
	captionTasks, err := getCaptionTasksByProjectID(projectID)
	if err != nil {
		status.Status = "error"
		status.Error = err.Error()
		updateExportStatus(projectID, status)
		sendExportProgress(projectID, ExportProgress{
			ProjectID:    projectID,
			ExportType:   "image-text-pairs",
			Status:       "error",
			ErrorMessage: err.Error(),
		})
		return
	}

	// Get all images for path lookup
	images, err := getImagesByProjectID(projectID)
	if err != nil {
		status.Status = "error"
		status.Error = err.Error()
		updateExportStatus(projectID, status)
		sendExportProgress(projectID, ExportProgress{
			ProjectID:    projectID,
			ExportType:   "image-text-pairs",
			Status:       "error",
			ErrorMessage: err.Error(),
		})
		return
	}

	// Create image lookup map
	imageMap := make(map[string]*Image)
	for i := range images {
		imageMap[images[i].ID] = &images[i]
	}

	// Count valid tasks for progress tracking
	validTasks := 0
	for _, task := range captionTasks {
		if !task.Skipped && task.Caption.Valid && imageMap[task.ImageID] != nil {
			validTasks++
		}
	}

	status.Total = validTasks
	updateExportStatus(projectID, status)

	// Create temporary export directory
	exportDir := filepath.Join("data", "exports", projectID+"-image-text-pairs")

	// Clean and create directory
	os.RemoveAll(exportDir)
	if err := os.MkdirAll(exportDir, 0755); err != nil {
		status.Status = "error"
		status.Error = err.Error()
		updateExportStatus(projectID, status)
		return
	}

	sendExportProgress(projectID, ExportProgress{
		ProjectID:  projectID,
		ExportType: "image-text-pairs",
		Step:       "converting_images",
		Status:     "processing",
		Total:      validTasks,
	})

	// Process completed caption tasks with worker pool
	exportCount := 0
	processedTasks := 0
	var exportCountMu sync.Mutex

	// Worker pool for file operations
	const numWorkers = 4
	taskChan := make(chan CaptionTask, validTasks)
	resultChan := make(chan bool, validTasks)

	// Start workers
	for i := 0; i < numWorkers; i++ {
		go func() {
			for task := range taskChan {
				exportCountMu.Lock()
				currentCount := exportCount
				exportCount++
				exportCountMu.Unlock()

				success := processTaskForImageTextPairs(task, imageMap, projectID, exportDir, currentCount)
				resultChan <- success
			}
		}()
	}

	// Send tasks to workers
	for _, task := range captionTasks {
		if !task.Skipped && task.Caption.Valid && imageMap[task.ImageID] != nil {
			taskChan <- task
		}
	}
	close(taskChan)

	// Collect results and update progress
	for i := 0; i < validTasks; i++ {
		<-resultChan
		processedTasks++
		status.Progress = processedTasks
		updateExportStatus(projectID, status)

		sendExportProgress(projectID, ExportProgress{
			ProjectID:  projectID,
			ExportType: "image-text-pairs",
			Step:       "converting_images",
			Progress:   processedTasks,
			Total:      validTasks,
			Status:     "processing",
		})
	}

	sendExportProgress(projectID, ExportProgress{
		ProjectID:  projectID,
		ExportType: "image-text-pairs",
		Step:       "creating_zip",
		Status:     "processing",
	})

	// Create ZIP archive with progress
	zipPath := filepath.Join("data", "exports", project.Name+"_image-text-pairs.zip")
	if err := createZipArchiveWithProgress(exportDir, zipPath, projectID); err != nil {
		status.Status = "error"
		status.Error = err.Error()
		updateExportStatus(projectID, status)
		sendExportProgress(projectID, ExportProgress{
			ProjectID:    projectID,
			ExportType:   "image-text-pairs",
			Status:       "error",
			ErrorMessage: err.Error(),
		})
		return
	}

	// Update final status
	status.Status = "completed"
	status.FilePath = zipPath
	status.CompletedAt = "2023-01-01T00:05:00Z" // You might want to use actual timestamp
	updateExportStatus(projectID, status)

	sendExportProgress(projectID, ExportProgress{
		ProjectID:  projectID,
		ExportType: "image-text-pairs",
		Status:     "completed",
		FilePath:   zipPath,
	})

	// Clean up temporary directory (but keep zip file for download)
	go func() {
		os.RemoveAll(exportDir)
	}()

	exportCountMu.Lock()
	finalExportCount := exportCount
	exportCountMu.Unlock()

	logger.Info("Image-text-pairs export completed",
		"project_id", projectID,
		"exported_pairs", finalExportCount)
}

func processTaskForImageTextPairs(task CaptionTask, imageMap map[string]*Image, projectID, exportDir string, exportCount int) bool {
	image := imageMap[task.ImageID]
	if image == nil {
		return false
	}

	// Generate sequential filename
	imageFileName := fmt.Sprintf("%d.png", exportCount+1)
	textFileName := fmt.Sprintf("%d.txt", exportCount+1)

	// Read and convert image to PNG
	sourceImagePath := filepath.Join("data", "projects", projectID, image.Path)
	destImagePath := filepath.Join(exportDir, imageFileName)

	if err := convertImageToPNG(sourceImagePath, destImagePath); err != nil {
		logger.Error("Failed to convert image to PNG", "error", err)
		return false
	}

	// Write caption text file
	textFilePath := filepath.Join(exportDir, textFileName)
	captionContent := []byte(task.Caption.String)

	if err := os.WriteFile(textFilePath, captionContent, 0644); err != nil {
		logger.Error("Failed to write caption file", "error", err)
		return false
	}

	return true
}

// Helper function to copy image with format preservation option
func convertImageToPNG(sourcePath, destPath string) error {
	return convertImageWithOptions(sourcePath, destPath, false)
}

// Helper function to copy image preserving original format (more efficient)
func copyImagePreservingFormat(sourcePath, destPath string) error {
	return convertImageWithOptions(sourcePath, destPath, true)
}

func convertImageWithOptions(sourcePath, destPath string, preserveFormat bool) error {
	// Check if source is already PNG and just copy it
	sourceExt := strings.ToLower(filepath.Ext(sourcePath))
	if sourceExt == ".png" {
		return copyFile(sourcePath, destPath)
	}

	// Check actual format
	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("failed to open source image: %v", err)
	}
	defer sourceFile.Close()

	// Peek at format without full decode
	_, format, err := image.DecodeConfig(sourceFile)
	if err != nil {
		return fmt.Errorf("failed to decode image config: %v", err)
	}

	// If it's already PNG format, just copy
	if format == "png" {
		return copyFile(sourcePath, destPath)
	}

	// For JPEG, handle based on preserveFormat option
	if format == "jpeg" {
		if preserveFormat {
			// Keep original extension and format for maximum size efficiency
			originalDestPath := strings.TrimSuffix(destPath, filepath.Ext(destPath)) + ".jpg"
			return copyFile(sourcePath, originalDestPath)
		} else {
			// Copy original JPEG with .png extension - keeps file size small
			// Most ML frameworks can handle this format mismatch
			return copyFileWithFormat(sourcePath, destPath, "jpeg")
		}
	}

	// Only do actual PNG conversion for WebP or other formats that benefit from it
	sourceFile.Seek(0, 0) // Reset file pointer

	// Decode image (supports JPEG, PNG, WebP)
	img, _, err := image.Decode(sourceFile)
	if err != nil {
		return fmt.Errorf("failed to decode image: %v", err)
	}

	// Create destination file
	destFile, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %v", err)
	}
	defer destFile.Close()

	// Use optimized PNG encoder settings
	encoder := &png.Encoder{
		CompressionLevel: png.DefaultCompression,
	}

	if err := encoder.Encode(destFile, img); err != nil {
		return fmt.Errorf("failed to encode PNG: %v", err)
	}

	return nil
}

// Copy file preserving original format (for size efficiency)
func copyFileWithFormat(src, dst, format string) error {
	return copyFile(src, dst)
}

// Helper function to copy files with optimized buffering
func copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()

	// Use larger buffer for better performance
	buffer := make([]byte, 64*1024) // 64KB buffer
	_, err = io.CopyBuffer(destination, source, buffer)
	return err
}

// Helper function to create ZIP archive with optimized streaming
func createZipArchive(sourceDir, zipPath string) error {
	zipFile, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	// Use buffered writer for better performance
	bufferedZipFile := make([]byte, 64*1024) // 64KB buffer

	return filepath.Walk(sourceDir, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(sourceDir, filePath)
		if err != nil {
			return err
		}

		// Create zip entry header with optimized compression
		header := &zip.FileHeader{
			Name:   relPath,
			Method: zip.Deflate, // Use deflate compression for better ratios
		}
		zipFileWriter, err := zipWriter.CreateHeader(header)
		if err != nil {
			return err
		}

		file, err := os.Open(filePath)
		if err != nil {
			return err
		}
		defer file.Close()

		// Use optimized copy with buffer
		_, err = io.CopyBuffer(zipFileWriter, file, bufferedZipFile)
		return err
	})
}

// Optimized version with progress tracking
func createZipArchiveWithProgress(sourceDir, zipPath, projectID string) error {
	// Count total files first for progress tracking
	totalFiles := 0
	err := filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			totalFiles++
		}
		return nil
	})
	if err != nil {
		return err
	}

	zipFile, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	processedFiles := 0
	buffer := make([]byte, 64*1024) // 64KB buffer

	return filepath.Walk(sourceDir, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(sourceDir, filePath)
		if err != nil {
			return err
		}

		// Send progress update
		processedFiles++
		sendExportProgress(projectID, ExportProgress{
			ProjectID:  projectID,
			ExportType: "zip",
			Step:       "compressing",
			Progress:   processedFiles,
			Total:      totalFiles,
			Status:     "processing",
			FilePath:   relPath,
		})

		// Create zip entry header with optimized compression
		header := &zip.FileHeader{
			Name:   relPath,
			Method: zip.Deflate, // Use deflate compression for better ratios
		}
		zipFileWriter, err := zipWriter.CreateHeader(header)
		if err != nil {
			return err
		}

		file, err := os.Open(filePath)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.CopyBuffer(zipFileWriter, file, buffer)
		return err
	})
}

func exportProgressHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	projectID := r.URL.Query().Get("projectId")
	if projectID == "" {
		http.Error(w, "Project ID is required", http.StatusBadRequest)
		return
	}

	// Set headers for SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Create progress channel for this client
	progressCh := make(chan ExportProgress, 100)
	exportProgressMu.Lock()
	exportProgressClients[projectID] = progressCh
	exportProgressMu.Unlock()

	// Clean up when client disconnects
	defer func() {
		exportProgressMu.Lock()
		delete(exportProgressClients, projectID)
		exportProgressMu.Unlock()
		close(progressCh)
	}()

	// Send events to client
	for {
		select {
		case update := <-progressCh:
			data, _ := json.Marshal(update)
			fmt.Fprintf(w, "data: %s\n\n", data)
			w.(http.Flusher).Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func getExportStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	projectID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/projects/"), "/export-status")
	if projectID == "" {
		http.Error(w, "Project ID is required", http.StatusBadRequest)
		return
	}

	status := getExportStatus(projectID)
	if status == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "none"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func downloadExportHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	projectID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/projects/"), "/export/download")
	if projectID == "" {
		http.Error(w, "Project ID is required", http.StatusBadRequest)
		return
	}

	status := getExportStatus(projectID)
	if status == nil || status.Status != "completed" || status.FilePath == "" {
		http.Error(w, "No completed export found", http.StatusNotFound)
		return
	}

	// Check if file exists
	if _, err := os.Stat(status.FilePath); os.IsNotExist(err) {
		http.Error(w, "Export file not found", http.StatusNotFound)
		return
	}

	// Get project info for filename
	project, err := getProject(projectID)
	if err != nil {
		http.Error(w, "Failed to get project", http.StatusInternalServerError)
		return
	}

	// Set appropriate headers
	filename := fmt.Sprintf("%s_%s.zip", project.Name, status.ExportType)
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))

	// Serve the file
	http.ServeFile(w, r, status.FilePath)
}

type ForkProjectRequest struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

func forkProjectHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	projectID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/projects/"), "/fork")
	if projectID == "" {
		http.Error(w, "Project ID is required", http.StatusBadRequest)
		return
	}

	// Check if source project exists
	sourceProject, err := getProject(projectID)
	if err != nil {
		http.Error(w, "Failed to get source project", http.StatusInternalServerError)
		logError(r.Context(), "Failed to get source project for fork", err, slog.String("project_id", projectID))
		return
	}
	if sourceProject == nil {
		http.Error(w, "Source project not found", http.StatusNotFound)
		return
	}

	// Parse fork request
	var req ForkProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate fork request
	if req.Name == "" {
		http.Error(w, "Project name is required", http.StatusBadRequest)
		return
	}
	if req.Version == "" {
		req.Version = "1.0"
	}

	// Create forked project
	forkedProject := Project{
		ID:              uuid.New().String(),
		Name:            req.Name,
		Version:         req.Version,
		PromptButtons:   sourceProject.PromptButtons,
		ParentProjectID: &sourceProject.ID,
	}

	if err := createProject(&forkedProject); err != nil {
		http.Error(w, "Failed to create forked project", http.StatusInternalServerError)
		logError(r.Context(), "Failed to create forked project", err, slog.String("source_project_id", projectID))
		return
	}

	// Get source images
	sourceImages, err := getImagesByProjectID(projectID)
	if err != nil {
		http.Error(w, "Failed to get source images", http.StatusInternalServerError)
		logError(r.Context(), "Failed to get source images for fork", err, slog.String("project_id", projectID))
		return
	}

	// Create forked project directory
	forkedImageDir := filepath.Join("data", "projects", forkedProject.ID, "images")
	if err := os.MkdirAll(forkedImageDir, 0755); err != nil {
		http.Error(w, "Failed to create forked project directory", http.StatusInternalServerError)
		logError(r.Context(), "Failed to create forked project directory", err)
		return
	}

	// Copy images and create new image records
	var forkedImages []Image
	for _, sourceImage := range sourceImages {
		// Copy image file
		sourceImagePath := filepath.Join("data", "projects", projectID, sourceImage.Path)
		forkedImagePath := filepath.Join("data", "projects", forkedProject.ID, sourceImage.Path)
		if err := copyFile(sourceImagePath, forkedImagePath); err != nil {
			logError(r.Context(), "Failed to copy image file", err,
				slog.String("source", sourceImagePath),
				slog.String("dest", forkedImagePath))
			continue
		}

		// Create new image record
		forkedImage := Image{
			ID:        uuid.New().String(),
			ProjectID: forkedProject.ID,
			Path:      sourceImage.Path,
			PHash:     sourceImage.PHash,
		}
		forkedImages = append(forkedImages, forkedImage)
	}

	// Store forked images in database
	if len(forkedImages) > 0 {
		if err := createImages(forkedImages); err != nil {
			http.Error(w, "Failed to store forked images", http.StatusInternalServerError)
			logError(r.Context(), "Failed to store forked images", err, slog.String("forked_project_id", forkedProject.ID))
			return
		}
	}

	// Get source tasks and copy them to forked project
	sourceTasks, err := getTasksByProjectID(projectID)
	if err != nil {
		logError(r.Context(), "Failed to get source tasks for fork", err, slog.String("project_id", projectID))
		// Don't fail the fork if tasks can't be copied, just log the error
	} else if len(sourceTasks) > 0 {
		// Create mapping from source image IDs to forked image IDs
		imageIDMap := make(map[string]string)
		for i, sourceImage := range sourceImages {
			if i < len(forkedImages) {
				imageIDMap[sourceImage.ID] = forkedImages[i].ID
			}
		}

		// Copy tasks with updated image IDs
		var forkedTasks []Task
		for _, sourceTask := range sourceTasks {
			// Map source image IDs to forked image IDs
			forkedImageAID, hasImageA := imageIDMap[sourceTask.ImageAID]
			if !hasImageA {
				continue // Skip task if image A doesn't exist in fork
			}

			forkedTask := Task{
				ID:        uuid.New().String(),
				ProjectID: forkedProject.ID,
				ImageAID:  forkedImageAID,
				ImageBId:  sourceTask.ImageBId,
				Prompt:    sourceTask.Prompt,
				Skipped:   sourceTask.Skipped,
			}

			// Update ImageBId if it exists and is mapped
			if sourceTask.ImageBId.Valid {
				if forkedImageBID, hasImageB := imageIDMap[sourceTask.ImageBId.String]; hasImageB {
					forkedTask.ImageBId = sql.NullString{String: forkedImageBID, Valid: true}
				} else {
					// If image B doesn't exist in fork, clear the selection but keep the prompt
					forkedTask.ImageBId = sql.NullString{Valid: false}
				}
			}

			// Map candidate B IDs
			var forkedCandidateIDs []string
			for _, candidateID := range sourceTask.CandidateBIds {
				if forkedCandidateID, hasCandidate := imageIDMap[candidateID]; hasCandidate {
					forkedCandidateIDs = append(forkedCandidateIDs, forkedCandidateID)
				}
			}
			forkedTask.CandidateBIds = forkedCandidateIDs

			forkedTasks = append(forkedTasks, forkedTask)
		}

		// Store forked tasks
		if len(forkedTasks) > 0 {
			for _, task := range forkedTasks {
				if err := createTask(&task); err != nil {
					logError(r.Context(), "Failed to create forked task", err,
						slog.String("task_id", task.ID),
						slog.String("forked_project_id", forkedProject.ID))
					// Continue with other tasks even if one fails
				}
			}
		}

		logInfo(r.Context(), "Project forked successfully",
			slog.String("source_project_id", projectID),
			slog.String("forked_project_id", forkedProject.ID),
			slog.String("forked_project_name", forkedProject.Name),
			slog.Int("images_copied", len(forkedImages)),
			slog.Int("tasks_copied", len(forkedTasks)))
	} else {
		logInfo(r.Context(), "Project forked successfully",
			slog.String("source_project_id", projectID),
			slog.String("forked_project_id", forkedProject.ID),
			slog.String("forked_project_name", forkedProject.Name),
			slog.Int("images_copied", len(forkedImages)),
			slog.Int("tasks_copied", 0))
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(forkedProject)
}

func autoCaptionTaskHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	taskID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/caption-tasks/"), "/auto-caption")
	if taskID == "" {
		http.Error(w, "Caption task ID is required", http.StatusBadRequest)
		return
	}

	// Get the task to find the project ID
	task, err := getCaptionTask(taskID)
	if err != nil {
		http.Error(w, "Failed to get caption task", http.StatusInternalServerError)
		logError(r.Context(), "Failed to get caption task for auto-caption", err, slog.String("task_id", taskID))
		return
	}
	if task == nil {
		http.Error(w, "Caption task not found", http.StatusNotFound)
		return
	}

	// Generate caption
	response, err := GenerateCaptionForTask(r.Context(), task.ProjectID, taskID)
	if err != nil {
		http.Error(w, "Failed to generate caption", http.StatusInternalServerError)
		logError(r.Context(), "Failed to generate caption", err, slog.String("task_id", taskID))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func startAutoCaptioningHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	projectID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/projects/"), "/auto-caption-batch")
	if projectID == "" {
		http.Error(w, "Project ID is required", http.StatusBadRequest)
		return
	}

	// Parse request body
	var req AutoCaptionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Use defaults if parsing fails
		req.Config = AutoCaptionConfig{
			RPM:             3, // 3 requests per minute
			MaxRetries:      3,
			RetryDelayMs:    10000,
			ConcurrentTasks: 1,
		}
	}

	// Validate config
	if req.Config.RPM <= 0 {
		req.Config.RPM = 3
	}
	if req.Config.MaxRetries <= 0 {
		req.Config.MaxRetries = 3
	}
	if req.Config.RetryDelayMs <= 0 {
		req.Config.RetryDelayMs = 10000
	}

	// Start auto captioning
	err := autoCaptionManager.StartAutoCaptioning(projectID, req.Config)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		logError(r.Context(), "Failed to start auto captioning", err, slog.String("project_id", projectID))
		return
	}

	logInfo(r.Context(), "Started auto captioning",
		slog.String("project_id", projectID),
		slog.Int("rpm", req.Config.RPM),
		slog.Int("max_retries", req.Config.MaxRetries),
	)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Auto captioning started",
		"config":  req.Config,
	})
}

func cancelAutoCaptioningHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	projectID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/projects/"), "/auto-caption-cancel")
	if projectID == "" {
		http.Error(w, "Project ID is required", http.StatusBadRequest)
		return
	}

	err := autoCaptionManager.CancelAutoCaptioning(projectID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		logError(r.Context(), "Failed to cancel auto captioning", err, slog.String("project_id", projectID))
		return
	}

	logInfo(r.Context(), "Cancelled auto captioning", slog.String("project_id", projectID))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Auto captioning cancelled",
	})
}

func getAutoCaptionStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	projectID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/projects/"), "/auto-caption-status")
	if projectID == "" {
		http.Error(w, "Project ID is required", http.StatusBadRequest)
		return
	}

	status, err := autoCaptionManager.GetAutoCaptionStatus(projectID)
	if err != nil {
		http.Error(w, "Failed to get auto caption status", http.StatusInternalServerError)
		logError(r.Context(), "Failed to get auto caption status", err, slog.String("project_id", projectID))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func autoCaptionProgressHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	projectID := r.URL.Query().Get("projectId")
	if projectID == "" {
		http.Error(w, "Project ID is required", http.StatusBadRequest)
		return
	}

	// Set headers for SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Create progress channel for this client
	progressCh := make(chan AutoCaptionProgress, 100)
	autoCaptionManager.AddProgressClient(projectID, progressCh)

	// Clean up when client disconnects
	defer func() {
		autoCaptionManager.RemoveProgressClient(projectID)
	}()

	// Send events to client
	for {
		select {
		case update := <-progressCh:
			data, _ := json.Marshal(update)
			fmt.Fprintf(w, "data: %s\n\n", data)
			w.(http.Flusher).Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func approveCaptionTaskHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "PUT" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	taskID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/caption-tasks/"), "/approve")
	if taskID == "" {
		http.Error(w, "Caption task ID is required", http.StatusBadRequest)
		return
	}

	// Get existing task
	task, err := getCaptionTask(taskID)
	if err != nil {
		http.Error(w, "Failed to get caption task", http.StatusInternalServerError)
		logError(r.Context(), "Failed to get caption task for approval", err, slog.String("task_id", taskID))
		return
	}
	if task == nil {
		http.Error(w, "Caption task not found", http.StatusNotFound)
		return
	}

	// Update status to reviewed/completed
	task.Status = "completed"
	if err := updateCaptionTask(task); err != nil {
		http.Error(w, "Failed to approve caption", http.StatusInternalServerError)
		logError(r.Context(), "Failed to approve caption task", err, slog.String("task_id", taskID))
		return
	}

	logInfo(r.Context(), "Caption task approved", slog.String("task_id", taskID))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(task)
}

func rejectCaptionTaskHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "PUT" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	taskID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/caption-tasks/"), "/reject")
	if taskID == "" {
		http.Error(w, "Caption task ID is required", http.StatusBadRequest)
		return
	}

	// Get existing task
	task, err := getCaptionTask(taskID)
	if err != nil {
		http.Error(w, "Failed to get caption task", http.StatusInternalServerError)
		logError(r.Context(), "Failed to get caption task for rejection", err, slog.String("task_id", taskID))
		return
	}
	if task == nil {
		http.Error(w, "Caption task not found", http.StatusNotFound)
		return
	}

	// Reset to pending status and clear caption
	task.Status = "pending"
	task.Caption = sql.NullString{Valid: false}
	if err := updateCaptionTask(task); err != nil {
		http.Error(w, "Failed to reject caption", http.StatusInternalServerError)
		logError(r.Context(), "Failed to reject caption task", err, slog.String("task_id", taskID))
		return
	}

	logInfo(r.Context(), "Caption task rejected", slog.String("task_id", taskID))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(task)
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*") // Allow all origins for now
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func main() {
	// Initialize logger
	if err := initLogger(); err != nil {
		fmt.Printf("Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	// Initialize database
	if err := initDatabase(); err != nil {
		logger.Error("Failed to initialize database", "error", err)
		os.Exit(1)
	}
	defer closeDatabase()

	mux := http.NewServeMux()
	mux.HandleFunc("/ping", pingHandler)
	mux.HandleFunc("/projects", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			createProjectHandler(w, r)
		case http.MethodGet:
			listProjectsHandler(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/projects/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/generate-tasks") && r.Method == http.MethodPost {
			generateTasksHandler(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/tasks") && r.Method == http.MethodGet {
			getTasksHandler(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/caption-tasks") && r.Method == http.MethodGet {
			getCaptionTasksHandler(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/fork") && r.Method == http.MethodPost {
			forkProjectHandler(w, r)
			return
		}
		if strings.Contains(r.URL.Path, "/images/") {
			if r.Method == http.MethodGet {
				serveImageHandler(w, r)
				return
			} else if r.Method == http.MethodDelete {
				deleteImageHandler(w, r)
				return
			}
		}
		if strings.HasSuffix(r.URL.Path, "/export/jsonl") && r.Method == http.MethodGet {
			exportJSONLHandler(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/export/ai-toolkit") && r.Method == http.MethodGet {
			exportAIToolkitHandler(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/export/image-text-pairs") && r.Method == http.MethodGet {
			exportImageTextPairsHandler(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/auto-caption-batch") && r.Method == http.MethodPost {
			startAutoCaptioningHandler(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/auto-caption-cancel") && r.Method == http.MethodPost {
			cancelAutoCaptioningHandler(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/auto-caption-status") && r.Method == http.MethodGet {
			getAutoCaptionStatusHandler(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/export-status") && r.Method == http.MethodGet {
			getExportStatusHandler(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/export/download") && r.Method == http.MethodGet {
			downloadExportHandler(w, r)
			return
		}
		switch r.Method {
		case http.MethodGet:
			getProjectHandler(w, r)
		case http.MethodPut:
			updateProjectHandler(w, r)
		case http.MethodDelete:
			deleteProjectHandler(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/upload", uploadHandler)
	mux.HandleFunc("/progress", progressHandler)
	mux.HandleFunc("/images", getImagesHandler)
	mux.HandleFunc("/tasks/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			getTaskHandler(w, r)
		case http.MethodPut:
			updateTaskHandler(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/caption-tasks/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/auto-caption") && r.Method == http.MethodPost {
			autoCaptionTaskHandler(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/approve") && r.Method == "PUT" {
			approveCaptionTaskHandler(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/reject") && r.Method == "PUT" {
			rejectCaptionTaskHandler(w, r)
			return
		}
		switch r.Method {
		case http.MethodGet:
			getCaptionTaskHandler(w, r)
		case http.MethodPut:
			updateCaptionTaskHandler(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/auto-caption-progress", autoCaptionProgressHandler)
	mux.HandleFunc("/export-progress", exportProgressHandler)

	logger.Info("Server starting", "port", 8080)
	if err := http.ListenAndServe(":8080", loggingMiddleware(corsMiddleware(mux))); err != nil {
		logger.Error("Server failed", "error", err)
		os.Exit(1)
	}
}
