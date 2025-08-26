import { useState, useEffect } from 'react'
import { updateProject, type Project, type CaptionAPIConfig, type AutoCaptionConfig } from '../api'

interface ProjectSettingsModalProps {
  project: Project
  isOpen: boolean
  onClose: () => void
  onProjectUpdated: (updatedProject: Project) => void
}

export function ProjectSettingsModal({ project, isOpen, onClose, onProjectUpdated }: ProjectSettingsModalProps) {
  const [formData, setFormData] = useState({
    name: project.name,
    version: project.version,
    promptButtons: project.promptButtons || [],
    systemPrompt: project.systemPrompt || '',
    captionApiProvider: '',
    captionApiKey: '',
    captionApiModel: '',
    autoCaptionRpm: 30,
    autoCaptionMaxRetries: 3,
    autoCaptionRetryDelayMs: 1000,
    autoCaptionConcurrentTasks: 1
  })
  const [newPromptButton, setNewPromptButton] = useState('')
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    if (project.captionApi) {
      try {
        const apiConfig: CaptionAPIConfig = JSON.parse(project.captionApi)
        setFormData(prev => ({
          ...prev,
          captionApiProvider: apiConfig.provider || '',
          captionApiKey: apiConfig.apiKey || '',
          captionApiModel: apiConfig.model || ''
        }))
      } catch (error) {
        console.error('Failed to parse caption API config:', error)
      }
    }
    
    if (project.autoCaptionConfig) {
      try {
        const autoCaptionConfig: AutoCaptionConfig = JSON.parse(project.autoCaptionConfig)
        setFormData(prev => ({
          ...prev,
          autoCaptionRpm: autoCaptionConfig.rpm || 30,
          autoCaptionMaxRetries: autoCaptionConfig.maxRetries || 3,
          autoCaptionRetryDelayMs: autoCaptionConfig.retryDelayMs || 1000,
          autoCaptionConcurrentTasks: autoCaptionConfig.concurrentTasks || 1
        }))
      } catch (error) {
        console.error('Failed to parse auto caption config:', error)
      }
    }
  }, [project.captionApi, project.autoCaptionConfig])

  const handleInputChange = (field: string, value: string | number) => {
    setFormData(prev => ({ ...prev, [field]: value }))
  }

  const addPromptButton = () => {
    if (newPromptButton.trim() && !formData.promptButtons.includes(newPromptButton.trim())) {
      setFormData(prev => ({
        ...prev,
        promptButtons: [...prev.promptButtons, newPromptButton.trim()]
      }))
      setNewPromptButton('')
    }
  }

  const removePromptButton = (index: number) => {
    setFormData(prev => ({
      ...prev,
      promptButtons: prev.promptButtons.filter((_, i) => i !== index)
    }))
  }

  const handleSave = async () => {
    setSaving(true)
    try {
      // Prepare caption API configuration
      let captionApi = null
      if (formData.captionApiProvider && formData.captionApiKey) {
        const apiConfig: CaptionAPIConfig = {
          provider: formData.captionApiProvider,
          apiKey: formData.captionApiKey,
          model: formData.captionApiModel || undefined
        }
        captionApi = JSON.stringify(apiConfig)
      }

      // Prepare auto caption configuration
      let autoCaptionConfig = null
      if (project.projectType === 'caption') {
        const config: AutoCaptionConfig = {
          rpm: Number(formData.autoCaptionRpm),
          maxRetries: Number(formData.autoCaptionMaxRetries),
          retryDelayMs: Number(formData.autoCaptionRetryDelayMs),
          concurrentTasks: Number(formData.autoCaptionConcurrentTasks)
        }
        autoCaptionConfig = JSON.stringify(config)
      }

      const updatedProject: Omit<Project, 'id'> = {
        name: formData.name,
        version: formData.version,
        promptButtons: formData.promptButtons,
        parentProjectId: project.parentProjectId,
        projectType: project.projectType,
        captionApi,
        systemPrompt: formData.systemPrompt || null,
        autoCaptionConfig
      }

      const response = await updateProject(project.id, updatedProject)
      onProjectUpdated(response.data)
      onClose()
    } catch (error) {
      console.error('Error updating project:', error)
      alert('Failed to update project')
    } finally {
      setSaving(false)
    }
  }

  if (!isOpen) return null

  return (
    <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
      <div className="bg-white dark:bg-gray-800 rounded-lg p-6 w-full max-w-2xl max-h-[90vh] overflow-y-auto">
        <h2 className="text-xl font-semibold text-gray-900 dark:text-white mb-6">Project Settings</h2>
        
        <div className="space-y-6">
          {/* Basic Project Info */}
          <div className="space-y-4">
            <h3 className="text-lg font-medium text-gray-900 dark:text-white">Basic Information</h3>
            
            <div>
              <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                Project Name
              </label>
              <input
                type="text"
                value={formData.name}
                onChange={(e) => handleInputChange('name', e.target.value)}
                className="w-full p-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white"
                placeholder="Enter project name"
              />
            </div>

            <div>
              <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                Version
              </label>
              <input
                type="text"
                value={formData.version}
                onChange={(e) => handleInputChange('version', e.target.value)}
                className="w-full p-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white"
                placeholder="e.g., 1.0, v2.1"
              />
            </div>
          </div>

          {/* Prompt Buttons */}
          <div className="space-y-4">
            <h3 className="text-lg font-medium text-gray-900 dark:text-white">Quick Prompt Buttons</h3>
            
            <div>
              <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                Add New Prompt Button
              </label>
              <div className="flex space-x-2">
                <input
                  type="text"
                  value={newPromptButton}
                  onChange={(e) => setNewPromptButton(e.target.value)}
                  className="flex-1 p-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white"
                  placeholder="Enter prompt text"
                  onKeyPress={(e) => e.key === 'Enter' && addPromptButton()}
                />
                <button
                  onClick={addPromptButton}
                  className="px-4 py-2 bg-indigo-600 text-white rounded-lg hover:bg-indigo-700"
                >
                  Add
                </button>
              </div>
            </div>

            {formData.promptButtons.length > 0 && (
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
                  Current Prompt Buttons
                </label>
                <div className="space-y-2">
                  {formData.promptButtons.map((button, index) => (
                    <div key={index} className="flex items-center justify-between p-2 border border-gray-200 dark:border-gray-600 rounded-lg">
                      <span className="text-gray-900 dark:text-white">{button}</span>
                      <button
                        onClick={() => removePromptButton(index)}
                        className="text-red-600 hover:text-red-800"
                      >
                        Remove
                      </button>
                    </div>
                  ))}
                </div>
              </div>
            )}
          </div>

          {/* Caption API Configuration (for both caption and edit projects) */}
          {(project.projectType === 'caption' || project.projectType === 'edit') && (
            <div className="space-y-4">
              <h3 className="text-lg font-medium text-gray-900 dark:text-white">
                {project.projectType === 'caption' ? 'Caption API Configuration' : 'Auto Captioning Configuration'}
              </h3>
              
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                  System Prompt
                </label>
                <textarea
                  value={formData.systemPrompt}
                  onChange={(e) => handleInputChange('systemPrompt', e.target.value)}
                  className="w-full p-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white min-h-20 resize-y"
                  placeholder={
                    project.projectType === 'caption' 
                      ? "Custom system prompt for AI captioning (e.g., 'Describe this image in detail for training a diffusion model')"
                      : "Custom system prompt for edit descriptions (e.g., 'Compare these images and describe the transformation that was applied')"
                  }
                  rows={3}
                />
                <p className="text-xs text-gray-500 dark:text-gray-400 mt-1">
                  {project.projectType === 'caption' 
                    ? 'This prompt will be used when generating automatic captions.'
                    : 'This prompt will be used when generating automatic edit descriptions.'
                  } You can resize this text area as needed.
                </p>
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                  API Provider
                </label>
                <select
                  value={formData.captionApiProvider}
                  onChange={(e) => handleInputChange('captionApiProvider', e.target.value)}
                  className="w-full p-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white"
                >
                  <option value="">Select API Provider</option>
                  <option value="gemini">Google Gemini</option>
                </select>
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                  API Key
                </label>
                <input
                  type="password"
                  value={formData.captionApiKey}
                  onChange={(e) => handleInputChange('captionApiKey', e.target.value)}
                  className="w-full p-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white"
                  placeholder="Enter your API key"
                />
                <p className="text-xs text-gray-500 dark:text-gray-400 mt-1">
                  Your API key is stored securely and only used for generating captions
                </p>
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                  Model (Optional)
                </label>
                <input
                  type="text"
                  value={formData.captionApiModel}
                  onChange={(e) => handleInputChange('captionApiModel', e.target.value)}
                  className="w-full p-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white"
                  placeholder="e.g., gemini-1.5-flash (optional)"
                />
              </div>

              {/* Auto Caption Configuration */}
              <div className="space-y-4 border-t border-gray-200 dark:border-gray-600 pt-4">
                <h4 className="text-md font-medium text-gray-900 dark:text-white">Auto Captioning Settings</h4>
                <p className="text-sm text-gray-600 dark:text-gray-400">
                  {project.projectType === 'caption' 
                    ? 'Configure bulk auto captioning behavior when processing multiple images at once.'
                    : 'Configure bulk auto caption behavior when processing multiple image pairs at once.'
                  }
                </p>
                
                <div className="grid grid-cols-2 gap-4">
                  <div>
                    <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                      Requests Per Minute (RPM)
                    </label>
                    <input
                      type="number"
                      min="1"
                      max="300"
                      value={formData.autoCaptionRpm}
                      onChange={(e) => handleInputChange('autoCaptionRpm', parseInt(e.target.value) || 30)}
                      className="w-full p-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white"
                    />
                    <p className="text-xs text-gray-500 dark:text-gray-400 mt-1">
                      Rate limit for API calls (1-300)
                    </p>
                  </div>

                  <div>
                    <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                      Max Retries
                    </label>
                    <input
                      type="number"
                      min="0"
                      max="10"
                      value={formData.autoCaptionMaxRetries}
                      onChange={(e) => handleInputChange('autoCaptionMaxRetries', parseInt(e.target.value) || 3)}
                      className="w-full p-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white"
                    />
                    <p className="text-xs text-gray-500 dark:text-gray-400 mt-1">
                      Retry attempts on failure (0-10)
                    </p>
                  </div>

                  <div>
                    <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                      Retry Delay (ms)
                    </label>
                    <input
                      type="number"
                      min="100"
                      max="10000"
                      step="100"
                      value={formData.autoCaptionRetryDelayMs}
                      onChange={(e) => handleInputChange('autoCaptionRetryDelayMs', parseInt(e.target.value) || 1000)}
                      className="w-full p-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white"
                    />
                    <p className="text-xs text-gray-500 dark:text-gray-400 mt-1">
                      Base delay between retries
                    </p>
                  </div>

                  <div>
                    <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                      Concurrent Tasks
                    </label>
                    <input
                      type="number" 
                      min="1"
                      max="5"
                      value={formData.autoCaptionConcurrentTasks}
                      onChange={(e) => handleInputChange('autoCaptionConcurrentTasks', parseInt(e.target.value) || 1)}
                      className="w-full p-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white"
                    />
                    <p className="text-xs text-gray-500 dark:text-gray-400 mt-1">
                      Parallel processing tasks (1-5)
                    </p>
                  </div>
                </div>

                <div className="bg-yellow-50 dark:bg-yellow-900/20 border border-yellow-200 dark:border-yellow-700 rounded-lg p-3">
                  <div className="flex">
                    <svg className="w-5 h-5 text-yellow-400 mr-2 mt-0.5" fill="currentColor" viewBox="0 0 20 20">
                      <path fillRule="evenodd" d="M8.257 3.099c.765-1.36 2.722-1.36 3.486 0l5.58 9.92c.75 1.334-.213 2.98-1.742 2.98H4.42c-1.53 0-2.493-1.646-1.743-2.98l5.58-9.92zM11 13a1 1 0 11-2 0 1 1 0 012 0zm-1-8a1 1 0 00-1 1v3a1 1 0 002 0V6a1 1 0 00-1-1z" clipRule="evenodd" />
                    </svg>
                    <div>
                      <h5 className="text-sm font-medium text-yellow-800 dark:text-yellow-200">API Rate Limits</h5>
                      <p className="text-sm text-yellow-700 dark:text-yellow-300">
                        Be mindful of your API provider's rate limits. Setting RPM too high may result in API errors or account suspension.
                      </p>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          )}
        </div>

        {/* Action Buttons */}
        <div className="flex justify-end space-x-3 mt-8">
          <button
            onClick={onClose}
            disabled={saving}
            className="px-4 py-2 border border-gray-300 dark:border-gray-600 text-gray-700 dark:text-gray-300 rounded-lg hover:bg-gray-50 dark:hover:bg-gray-700 disabled:opacity-50"
          >
            Cancel
          </button>
          <button
            onClick={handleSave}
            disabled={saving || !formData.name.trim()}
            className="px-4 py-2 bg-indigo-600 text-white rounded-lg hover:bg-indigo-700 disabled:opacity-50"
          >
            {saving ? 'Saving...' : 'Save Changes'}
          </button>
        </div>
      </div>
    </div>
  )
}