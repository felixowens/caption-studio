-- +goose Up
-- +goose StatementBegin

CREATE TABLE projects (
  id           TEXT PRIMARY KEY,
  name         TEXT NOT NULL,
  kind         TEXT NOT NULL DEFAULT 'EDIT'
                CHECK (kind IN ('EDIT','CAPTION')),
  parent_id    TEXT REFERENCES projects(id) ON DELETE SET NULL, -- If the project was forked, the parent_id is the original project
  caption_provider TEXT,
  system_prompt    TEXT,
  auto_caption_config TEXT,
  created_at   TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
  updated_at   TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
) STRICT;

CREATE INDEX idx_projects_parent ON projects(parent_id);
CREATE INDEX idx_projects_kind   ON projects(kind);

CREATE TABLE images (
  id              TEXT PRIMARY KEY,
  project_id      TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  rel_path        TEXT NOT NULL,
  file_hash       TEXT NOT NULL,
  phash           TEXT,
  width           INTEGER,
  height          INTEGER,
  filesize_bytes  INTEGER,
  metadata_json   TEXT,
  caption         TEXT,
  created_at      TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
  updated_at      TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
  UNIQUE (project_id, rel_path),
  UNIQUE (project_id, file_hash)
) STRICT;

CREATE INDEX idx_images_project        ON images(project_id);
CREATE INDEX idx_images_project_phash  ON images(project_id, phash);
CREATE INDEX idx_images_created        ON images(project_id, created_at);

CREATE TABLE tasks (
  id           TEXT PRIMARY KEY,
  project_id   TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  type         TEXT NOT NULL CHECK (type IN ('EDIT','CAPTION')),
  status       TEXT NOT NULL DEFAULT 'PENDING'
               CHECK (status IN ('PENDING','QUEUED','RUNNING','COMPLETED','SKIPPED','FAILED','CANCELED')),
  priority     INTEGER NOT NULL DEFAULT 0,
  prompt       TEXT,
  result_text  TEXT,
  error        TEXT,
  scheduled_at TEXT,
  started_at   TEXT,
  finished_at  TEXT,
  created_at   TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
  updated_at   TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
) STRICT;

CREATE INDEX idx_tasks_project_status ON tasks(project_id, status);
CREATE INDEX idx_tasks_project_type   ON tasks(project_id, type);
CREATE INDEX idx_tasks_priority       ON tasks(project_id, status, priority DESC, created_at);

CREATE TABLE task_images (
  id        INTEGER PRIMARY KEY AUTOINCREMENT,
  task_id   TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  image_id  TEXT NOT NULL REFERENCES images(id) ON DELETE CASCADE,
  role      TEXT NOT NULL CHECK (role IN ('A','B','CANDIDATE','TARGET')),
  position  INTEGER NOT NULL DEFAULT 0,
  UNIQUE (task_id, image_id, role)
) STRICT;

CREATE UNIQUE INDEX uniq_task_role_a       ON task_images(task_id) WHERE role='A';
CREATE UNIQUE INDEX uniq_task_role_b       ON task_images(task_id) WHERE role='B';
CREATE UNIQUE INDEX uniq_task_role_target  ON task_images(task_id) WHERE role='TARGET';

CREATE INDEX idx_task_candidates ON task_images(task_id, position) WHERE role='CANDIDATE';

CREATE TABLE task_events (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  task_id     TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  prev_status TEXT CHECK (prev_status IN ('PENDING','QUEUED','RUNNING','COMPLETED','SKIPPED','FAILED','CANCELED')),
  new_status  TEXT NOT NULL CHECK (new_status  IN ('PENDING','QUEUED','RUNNING','COMPLETED','SKIPPED','FAILED','CANCELED')),
  message     TEXT,
  created_at  TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
) STRICT;

CREATE INDEX idx_task_events_task ON task_events(task_id, created_at);

CREATE VIEW edit_tasks_expanded AS
SELECT
  t.*,
  (SELECT image_id FROM task_images ti WHERE ti.task_id = t.id AND ti.role = 'A') AS image_a_id,
  (SELECT image_id FROM task_images ti WHERE ti.task_id = t.id AND ti.role = 'B') AS image_b_id
FROM tasks t
WHERE t.type = 'EDIT';

CREATE VIEW caption_tasks_expanded AS
SELECT
  t.*,
  (SELECT image_id FROM task_images ti WHERE ti.task_id = t.id AND ti.role = 'TARGET') AS image_id
FROM tasks t
WHERE t.type = 'CAPTION';

CREATE TRIGGER projects_touch_updated_at
AFTER UPDATE ON projects
BEGIN
  UPDATE projects SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

CREATE TRIGGER images_touch_updated_at
AFTER UPDATE ON images
BEGIN
  UPDATE images SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

CREATE TRIGGER tasks_touch_updated_at
AFTER UPDATE ON tasks
BEGIN
  UPDATE tasks SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP VIEW edit_tasks_expanded;
DROP VIEW caption_tasks_expanded;
DROP TABLE task_events;
DROP TABLE task_images;
DROP TABLE tasks;
DROP TABLE images;
DROP TABLE projects;
-- +goose StatementEnd
