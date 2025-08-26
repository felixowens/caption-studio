package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var db *sql.DB

func initDatabase() error {
	// Ensure data directory exists
	dataDir := "data"
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %v", err)
	}

	// Open database connection
	dbPath := filepath.Join(dataDir, "app.db")
	var err error
	db, err = sql.Open("sqlite3", dbPath+"?_foreign_keys=on")
	if err != nil {
		return fmt.Errorf("failed to open database: %v", err)
	}

	// Set connection pool settings
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Test connection
	if err := db.Ping(); err != nil {
		return fmt.Errorf("failed to ping database: %v", err)
	}

	// Run migrations
	if err := runMigrations(); err != nil {
		return fmt.Errorf("failed to run migrations: %v", err)
	}

	logger.Info("Database initialized successfully", 
		"db_path", dbPath,
		"max_connections", 25,
	)
	return nil
}

func runMigrations() error {
	// Create schema version table
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_version (
			version INTEGER PRIMARY KEY,
			applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create schema_version table: %v", err)
	}

	// Get current schema version
	var currentVersion int
	err = db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_version").Scan(&currentVersion)
	if err != nil {
		return fmt.Errorf("failed to get current schema version: %v", err)
	}

	// Run migrations
	migrations := []migration{
		{1, createInitialTables},
		{2, addPromptButtonsToProjects},
		{3, addImagePathConstraint},
		{4, addParentProjectIdToProjects},
		{5, addProjectTypeSupport},
		{6, addCaptionAPISupport},
		{7, addAutoCaptionSupport},
		{8, addStatusToTasks},
	}

	for _, m := range migrations {
		if m.version > currentVersion {
			logger.Info("Running database migration", "version", m.version)
			if err := m.up(); err != nil {
				return fmt.Errorf("migration %d failed: %v", m.version, err)
			}

			// Record migration
			_, err = db.Exec("INSERT INTO schema_version (version) VALUES (?)", m.version)
			if err != nil {
				return fmt.Errorf("failed to record migration %d: %v", m.version, err)
			}
			logger.Info("Migration completed successfully", "version", m.version)
		}
	}

	return nil
}

type migration struct {
	version int
	up      func() error
}

func createInitialTables() error {
	queries := []string{
		`CREATE TABLE projects (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			version TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE images (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL,
			path TEXT NOT NULL,
			phash TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE tasks (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL,
			image_a_id TEXT NOT NULL,
			image_b_id TEXT,
			prompt TEXT,
			skipped BOOLEAN DEFAULT FALSE,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
			FOREIGN KEY (image_a_id) REFERENCES images(id) ON DELETE CASCADE,
			FOREIGN KEY (image_b_id) REFERENCES images(id) ON DELETE SET NULL
		)`,
		`CREATE TABLE task_candidates (
			task_id TEXT NOT NULL,
			image_id TEXT NOT NULL,
			PRIMARY KEY (task_id, image_id),
			FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE,
			FOREIGN KEY (image_id) REFERENCES images(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX idx_images_project_id ON images(project_id)`,
		`CREATE INDEX idx_images_phash ON images(phash)`,
		`CREATE INDEX idx_tasks_project_id ON tasks(project_id)`,
		`CREATE INDEX idx_tasks_image_a_id ON tasks(image_a_id)`,
		`CREATE INDEX idx_task_candidates_task_id ON task_candidates(task_id)`,
	}

	for _, query := range queries {
		if _, err := db.Exec(query); err != nil {
			return fmt.Errorf("failed to execute query: %s - %v", query, err)
		}
	}

	return nil
}

func addPromptButtonsToProjects() error {
	_, err := db.Exec(`ALTER TABLE projects ADD COLUMN prompt_buttons TEXT DEFAULT '[]'`)
	return err
}

func addImagePathConstraint() error {
	_, err := db.Exec(`CREATE UNIQUE INDEX idx_images_project_path ON images(project_id, path)`)
	return err
}

func addParentProjectIdToProjects() error {
	_, err := db.Exec(`ALTER TABLE projects ADD COLUMN parent_project_id TEXT REFERENCES projects(id)`)
	return err
}

func addProjectTypeSupport() error {
	queries := []string{
		// Add project_type column with default 'edit' for existing projects
		`ALTER TABLE projects ADD COLUMN project_type TEXT DEFAULT 'edit' NOT NULL`,
		// Create caption_tasks table for single-image captioning
		`CREATE TABLE caption_tasks (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL,
			image_id TEXT NOT NULL,
			caption TEXT,
			skipped BOOLEAN DEFAULT FALSE,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
			FOREIGN KEY (image_id) REFERENCES images(id) ON DELETE CASCADE
		)`,
		// Add indexes for caption_tasks
		`CREATE INDEX idx_caption_tasks_project_id ON caption_tasks(project_id)`,
		`CREATE INDEX idx_caption_tasks_image_id ON caption_tasks(image_id)`,
		// Add unique constraint to ensure one caption task per image per project
		`CREATE UNIQUE INDEX idx_caption_tasks_project_image ON caption_tasks(project_id, image_id)`,
	}

	for _, query := range queries {
		if _, err := db.Exec(query); err != nil {
			return fmt.Errorf("failed to execute query: %s - %v", query, err)
		}
	}

	return nil
}

func addCaptionAPISupport() error {
	queries := []string{
		// Add caption_api and system_prompt columns to projects table
		`ALTER TABLE projects ADD COLUMN caption_api TEXT`,
		`ALTER TABLE projects ADD COLUMN system_prompt TEXT`,
	}

	for _, query := range queries {
		if _, err := db.Exec(query); err != nil {
			return fmt.Errorf("failed to execute query: %s - %v", query, err)
		}
	}

	return nil
}

// Project database operations
func createProject(project *Project) error {
	promptButtonsJSON, err := json.Marshal(project.PromptButtons)
	if err != nil {
		return fmt.Errorf("failed to marshal prompt buttons: %v", err)
	}
	_, err = db.Exec(
		"INSERT INTO projects (id, name, version, prompt_buttons, parent_project_id, project_type, caption_api, system_prompt, auto_caption_config) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		project.ID, project.Name, project.Version, string(promptButtonsJSON), project.ParentProjectID, project.ProjectType, project.CaptionAPI, project.SystemPrompt, project.AutoCaptionConfig,
	)
	return err
}

func getProject(id string) (*Project, error) {
	var project Project
	var promptButtonsJSON string
	err := db.QueryRow(
		"SELECT id, name, version, COALESCE(prompt_buttons, '[]'), parent_project_id, COALESCE(project_type, 'edit'), caption_api, system_prompt, auto_caption_config FROM projects WHERE id = ?", id,
	).Scan(&project.ID, &project.Name, &project.Version, &promptButtonsJSON, &project.ParentProjectID, &project.ProjectType, &project.CaptionAPI, &project.SystemPrompt, &project.AutoCaptionConfig)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal([]byte(promptButtonsJSON), &project.PromptButtons); err != nil {
		return nil, fmt.Errorf("failed to unmarshal prompt buttons: %v", err)
	}

	return &project, nil
}

func listProjects() ([]Project, error) {
	rows, err := db.Query("SELECT id, name, version, COALESCE(prompt_buttons, '[]'), parent_project_id, COALESCE(project_type, 'edit'), caption_api, system_prompt, auto_caption_config FROM projects ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []Project
	for rows.Next() {
		var project Project
		var promptButtonsJSON string
		if err := rows.Scan(&project.ID, &project.Name, &project.Version, &promptButtonsJSON, &project.ParentProjectID, &project.ProjectType, &project.CaptionAPI, &project.SystemPrompt, &project.AutoCaptionConfig); err != nil {
			return nil, err
		}
		
		if err := json.Unmarshal([]byte(promptButtonsJSON), &project.PromptButtons); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prompt buttons: %v", err)
		}
		
		projects = append(projects, project)
	}

	return projects, rows.Err()
}

func updateProject(project *Project) error {
	promptButtonsJSON, err := json.Marshal(project.PromptButtons)
	if err != nil {
		return fmt.Errorf("failed to marshal prompt buttons: %v", err)
	}
	_, err = db.Exec(
		"UPDATE projects SET name = ?, version = ?, prompt_buttons = ?, parent_project_id = ?, project_type = ?, caption_api = ?, system_prompt = ?, auto_caption_config = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?",
		project.Name, project.Version, string(promptButtonsJSON), project.ParentProjectID, project.ProjectType, project.CaptionAPI, project.SystemPrompt, project.AutoCaptionConfig, project.ID,
	)
	return err
}

func deleteProject(id string) error {
	_, err := db.Exec("DELETE FROM projects WHERE id = ?", id)
	return err
}

// Image database operations
func createImage(image *Image) error {
	_, err := db.Exec(
		"INSERT INTO images (id, project_id, path, phash) VALUES (?, ?, ?, ?)",
		image.ID, image.ProjectID, image.Path, image.PHash,
	)
	return err
}

func createImages(images []Image) error {
	if len(images) == 0 {
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare("INSERT INTO images (id, project_id, path, phash) VALUES (?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, image := range images {
		if _, err := stmt.Exec(image.ID, image.ProjectID, image.Path, image.PHash); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func getImagesByProjectID(projectID string) ([]Image, error) {
	rows, err := db.Query(
		"SELECT id, project_id, path, phash FROM images WHERE project_id = ? ORDER BY created_at",
		projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var images []Image
	for rows.Next() {
		var image Image
		if err := rows.Scan(&image.ID, &image.ProjectID, &image.Path, &image.PHash); err != nil {
			return nil, err
		}
		images = append(images, image)
	}

	return images, rows.Err()
}

func getImage(id string) (*Image, error) {
	var image Image
	err := db.QueryRow(
		"SELECT id, project_id, path, phash FROM images WHERE id = ?", id,
	).Scan(&image.ID, &image.ProjectID, &image.Path, &image.PHash)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &image, nil
}

func imageExistsByPath(projectID, path string) (bool, error) {
	var count int
	err := db.QueryRow(
		"SELECT COUNT(*) FROM images WHERE project_id = ? AND path = ?",
		projectID, path,
	).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func imageExistsByHash(projectID, hash string, threshold int) (bool, error) {
	rows, err := db.Query(
		"SELECT phash FROM images WHERE project_id = ?",
		projectID,
	)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var existingHash string
		if err := rows.Scan(&existingHash); err != nil {
			continue
		}

		// Compare hashes (this is a simplified check - in production you'd use proper hash comparison)
		if existingHash == hash {
			return true, nil
		}
	}

	return false, rows.Err()
}

func deleteImage(imageID string) error {
	_, err := db.Exec("DELETE FROM images WHERE id = ?", imageID)
	return err
}

// Task database operations
func createTask(task *Task) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Insert task
	query := "INSERT INTO tasks (id, project_id, image_a_id, image_b_id, prompt, status, skipped) VALUES (?, ?, ?, ?, ?, ?, ?)"
	_, err = tx.Exec(query, task.ID, task.ProjectID, task.ImageAID, task.ImageBId, task.Prompt, task.Status, task.Skipped)
	if err != nil {
		return err
	}

	// Insert candidate B images
	if len(task.CandidateBIds) > 0 {
		stmt, err := tx.Prepare("INSERT INTO task_candidates (task_id, image_id) VALUES (?, ?)")
		if err != nil {
			return err
		}
		defer stmt.Close()

		for _, candidateID := range task.CandidateBIds {
			if _, err := stmt.Exec(task.ID, candidateID); err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

func getTasksByProjectID(projectID string) ([]Task, error) {
	rows, err := db.Query(`
		SELECT id, project_id, image_a_id, image_b_id, prompt, status, skipped 
		FROM tasks 
		WHERE project_id = ? 
		ORDER BY created_at
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		var task Task
		if err := rows.Scan(&task.ID, &task.ProjectID, &task.ImageAID, &task.ImageBId, &task.Prompt, &task.Status, &task.Skipped); err != nil {
			return nil, err
		}

		// Get candidate B IDs
		candidateRows, err := db.Query("SELECT image_id FROM task_candidates WHERE task_id = ?", task.ID)
		if err != nil {
			return nil, err
		}

		var candidateIDs []string
		for candidateRows.Next() {
			var candidateID string
			if err := candidateRows.Scan(&candidateID); err != nil {
				candidateRows.Close()
				return nil, err
			}
			candidateIDs = append(candidateIDs, candidateID)
		}
		candidateRows.Close()

		task.CandidateBIds = candidateIDs
		tasks = append(tasks, task)
	}

	return tasks, rows.Err()
}

func updateTask(task *Task) error {
	_, err := db.Exec(
		"UPDATE tasks SET image_b_id = ?, prompt = ?, status = ?, skipped = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?",
		task.ImageBId, task.Prompt, task.Status, task.Skipped, task.ID,
	)
	return err
}

func taskExistsForImageA(projectID, imageAID string) (bool, error) {
	var count int
	err := db.QueryRow(
		"SELECT COUNT(*) FROM tasks WHERE project_id = ? AND image_a_id = ?",
		projectID, imageAID,
	).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func getTask(id string) (*Task, error) {
	var task Task
	err := db.QueryRow(`
		SELECT id, project_id, image_a_id, image_b_id, prompt, status, skipped 
		FROM tasks 
		WHERE id = ?
	`, id).Scan(&task.ID, &task.ProjectID, &task.ImageAID, &task.ImageBId, &task.Prompt, &task.Status, &task.Skipped)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// Get candidate B IDs
	rows, err := db.Query("SELECT image_id FROM task_candidates WHERE task_id = ?", task.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var candidateIDs []string
	for rows.Next() {
		var candidateID string
		if err := rows.Scan(&candidateID); err != nil {
			return nil, err
		}
		candidateIDs = append(candidateIDs, candidateID)
	}

	task.CandidateBIds = candidateIDs
	return &task, nil
}

// Caption Task database operations
func createCaptionTask(task *CaptionTask) error {
	_, err := db.Exec(
		"INSERT INTO caption_tasks (id, project_id, image_id, caption, status, skipped) VALUES (?, ?, ?, ?, ?, ?)",
		task.ID, task.ProjectID, task.ImageID, task.Caption, task.Status, task.Skipped,
	)
	return err
}

func createCaptionTasks(tasks []CaptionTask) error {
	if len(tasks) == 0 {
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare("INSERT INTO caption_tasks (id, project_id, image_id, caption, status, skipped) VALUES (?, ?, ?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, task := range tasks {
		if _, err := stmt.Exec(task.ID, task.ProjectID, task.ImageID, task.Caption, task.Status, task.Skipped); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func getCaptionTasksByProjectID(projectID string) ([]CaptionTask, error) {
	rows, err := db.Query(`
		SELECT id, project_id, image_id, caption, status, skipped 
		FROM caption_tasks 
		WHERE project_id = ? 
		ORDER BY created_at
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []CaptionTask
	for rows.Next() {
		var task CaptionTask
		if err := rows.Scan(&task.ID, &task.ProjectID, &task.ImageID, &task.Caption, &task.Status, &task.Skipped); err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}

	return tasks, rows.Err()
}

func getCaptionTask(id string) (*CaptionTask, error) {
	var task CaptionTask
	err := db.QueryRow(`
		SELECT id, project_id, image_id, caption, status, skipped 
		FROM caption_tasks 
		WHERE id = ?
	`, id).Scan(&task.ID, &task.ProjectID, &task.ImageID, &task.Caption, &task.Status, &task.Skipped)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &task, nil
}

func updateCaptionTask(task *CaptionTask) error {
	_, err := db.Exec(
		"UPDATE caption_tasks SET caption = ?, status = ?, skipped = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?",
		task.Caption, task.Status, task.Skipped, task.ID,
	)
	return err
}

func captionTaskExistsForImage(projectID, imageID string) (bool, error) {
	var count int
	err := db.QueryRow(
		"SELECT COUNT(*) FROM caption_tasks WHERE project_id = ? AND image_id = ?",
		projectID, imageID,
	).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func addAutoCaptionSupport() error {
	queries := []string{
		// Add auto_caption_config column to projects table
		`ALTER TABLE projects ADD COLUMN auto_caption_config TEXT`,
		// Add status column to caption_tasks table  
		`ALTER TABLE caption_tasks ADD COLUMN status TEXT DEFAULT 'pending'`,
		// Update existing tasks to have 'completed' status if they have a caption
		`UPDATE caption_tasks SET status = 'completed' WHERE caption IS NOT NULL AND caption != ''`,
	}
	for _, query := range queries {
		if _, err := db.Exec(query); err != nil {
			return fmt.Errorf("failed to execute query: %s - %v", query, err)
		}
	}
	return nil
}

func addStatusToTasks() error {
	queries := []string{
		// Add status column to tasks table for edit project auto captioning
		`ALTER TABLE tasks ADD COLUMN status TEXT DEFAULT 'pending'`,
		// Update existing tasks to have 'completed' status if they have a prompt
		`UPDATE tasks SET status = 'completed' WHERE prompt IS NOT NULL AND prompt != ''`,
	}
	for _, query := range queries {
		if _, err := db.Exec(query); err != nil {
			return fmt.Errorf("failed to execute query: %s - %v", query, err)
		}
	}
	return nil
}

func closeDatabase() error {
	if db != nil {
		return db.Close()
	}
	return nil
}
