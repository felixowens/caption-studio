import axios from 'axios';

const API_BASE_URL = 'http://localhost:8080';

export const api = axios.create({
  baseURL: API_BASE_URL,
  timeout: 100000,
});

export const ping = () => api.get('/ping');

export interface Project {
  id: string;
  name: string;
  version: string;
  promptButtons: string[];
  parentProjectId?: string | null;
  projectType: 'edit' | 'caption';
  captionApi?: string | null;
  systemPrompt?: string | null;
  autoCaptionConfig?: string | null;
}

export interface ProjectWithStats {
  id: string;
  name: string;
  version: string;
  promptButtons: string[];
  imageCount: number;
  taskCount: number;
  completedTaskCount: number;
  createdAt?: string;
  parentProjectId?: string | null;
  projectType: 'edit' | 'caption';
  captionApi?: string | null;
  systemPrompt?: string | null;
  autoCaptionConfig?: string | null;
}

export const createProject = (project: Omit<Project, 'id'>) => api.post<Project>('/projects', project);
export const getProject = (id: string) => api.get<Project>(`/projects/${id}`);
export const listProjects = () => api.get<Project[]>('/projects');
export const listProjectsWithStats = () => api.get<ProjectWithStats[]>('/projects/stats');
export const updateProject = (id: string, project: Omit<Project, 'id'>) => api.put<Project>(`/projects/${id}`, project);
export const deleteProject = (id: string) => api.delete(`/projects/${id}`);

export interface Image {
  id: string;
  projectId: string;
  path: string;
  pHash: string;
}

export interface ProgressUpdate {
  projectId: string;
  filename: string;
  progress: number;
  total: number;
  status: string;
  errorMessage?: string;
}

export const uploadFiles = (projectId: string, files: FileList) => {
  const formData = new FormData();
  Array.from(files).forEach(file => {
    formData.append('files', file);
  });

  return api.post(`/upload?projectId=${projectId}`, formData, {
    headers: {
      'Content-Type': 'multipart/form-data'
    }
  });
};

export const getImages = (projectId: string) => api.get<Image[]>(`/images?projectId=${projectId}`);
export const deleteImage = (projectId: string, imageId: string) => api.delete(`/projects/${projectId}/images/${imageId}`);

export const createProgressEventSource = (projectId: string) => {
  return new EventSource(`${API_BASE_URL}/progress?projectId=${projectId}`);
};

export interface Task {
  id: string;
  projectId: string;
  imageAId: string;
  imageBId: { String: string; Valid: boolean } | null;
  prompt: { String: string; Valid: boolean } | null;
  status: string; // "pending", "auto_generated", "reviewed", "completed"
  skipped: boolean;
  candidateBIds: string[] | null;
}

export interface CaptionTask {
  id: string;
  projectId: string;
  imageId: string;
  caption: { String: string; Valid: boolean } | null;
  status: 'pending' | 'auto_generated' | 'reviewed' | 'completed';
  skipped: boolean;
}

export interface TaskGenerationRequest {
  similarityThreshold?: number;
  maxCandidates?: number;
}

export interface TaskGenerationResponse {
  tasksCreated: number;
  averageCandidates: number;
}

export const generateTasks = (projectId: string, request?: TaskGenerationRequest) =>
  api.post<TaskGenerationResponse>(`/projects/${projectId}/generate-tasks`, request || {});

export const getTasks = (projectId: string) => api.get<Task[]>(`/projects/${projectId}/tasks`);
export const getTask = (taskId: string) => api.get<Task>(`/tasks/${taskId}`);
export const updateTask = (taskId: string, task: Partial<Task>) => api.put<Task>(`/tasks/${taskId}`, task);

export const getCaptionTasks = (projectId: string) => api.get<CaptionTask[]>(`/projects/${projectId}/caption-tasks`);
export const getCaptionTask = (taskId: string) => api.get<CaptionTask>(`/caption-tasks/${taskId}`);
export const updateCaptionTask = (taskId: string, task: Partial<CaptionTask>) => api.put<CaptionTask>(`/caption-tasks/${taskId}`, task);

export interface ForkProjectRequest {
  name: string;
  version: string;
}

export const forkProject = (sourceProjectId: string, forkData: ForkProjectRequest) => 
  api.post<Project>(`/projects/${sourceProjectId}/fork`, forkData);

export interface CaptionAPIConfig {
  provider: string;
  apiKey: string;
  endpoint?: string;
  model?: string;
}

export interface AutoCaptionResponse {
  caption?: string;
  error?: string;
}

export interface AutoCaptionConfig {
  rpm: number;
  maxRetries: number;
  retryDelayMs: number;
  concurrentTasks: number;
}

export interface AutoCaptionProgress {
  projectId: string;
  status: 'running' | 'completed' | 'cancelled' | 'error';
  total: number;
  processed: number;
  successful: number;
  failed: number;
  currentTask?: string;
  errorMessage?: string;
  startedAt?: string;
  completedAt?: string;
}

export interface AutoCaptionRequest {
  config: AutoCaptionConfig;
}

export interface AutoCaptionStatusResponse {
  progress?: AutoCaptionProgress;
  isActive: boolean;
}

export const autoCaptionTask = (taskId: string) => 
  api.post<AutoCaptionResponse>(`/caption-tasks/${taskId}/auto-caption`);

export const startAutoCaptioning = (projectId: string, config: AutoCaptionConfig) =>
  api.post<{ message: string; config: AutoCaptionConfig }>(`/projects/${projectId}/auto-caption-batch`, { config });

export const cancelAutoCaptioning = (projectId: string) =>
  api.post<{ message: string }>(`/projects/${projectId}/auto-caption-cancel`);

export const getAutoCaptionStatus = (projectId: string) =>
  api.get<AutoCaptionStatusResponse>(`/projects/${projectId}/auto-caption-status`);

export const createAutoCaptionProgressEventSource = (projectId: string) => {
  return new EventSource(`${API_BASE_URL}/auto-caption-progress?projectId=${projectId}`);
};

export const approveCaptionTask = (taskId: string) =>
  api.put<CaptionTask>(`/caption-tasks/${taskId}/approve`);

export const rejectCaptionTask = (taskId: string) =>
  api.put<CaptionTask>(`/caption-tasks/${taskId}/reject`);

// Export related APIs
export interface ExportProgress {
  projectId: string;
  exportType: string;
  step: string;
  progress: number;
  total: number;
  status: string;
  errorMessage?: string;
  filePath?: string;
}

export interface ExportStatus {
  projectId: string;
  exportType: string;
  status: string;
  progress: number;
  total: number;
  filePath?: string;
  error?: string;
  startTime: string;
  completedAt?: string;
}

export const startExport = (projectId: string, exportType: 'ai-toolkit' | 'image-text-pairs') =>
  api.get<{ message: string; projectId: string; type: string }>(`/projects/${projectId}/export/${exportType}`);

export const getExportStatus = (projectId: string) =>
  api.get<ExportStatus>(`/projects/${projectId}/export-status`);

export const downloadExport = (projectId: string) => {
  // Create a direct download link
  window.location.href = `${API_BASE_URL}/projects/${projectId}/export/download`;
};

export const createExportProgressEventSource = (projectId: string) => {
  return new EventSource(`${API_BASE_URL}/export-progress?projectId=${projectId}`);
};