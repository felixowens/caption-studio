import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { getCaptionTasks, getImages, updateCaptionTask, getProject, autoCaptionTask, approveCaptionTask, rejectCaptionTask, type CaptionTask, type Image, type Project } from '../api'

interface CaptionAnnotationWizardProps {
  projectId: string
}

export function CaptionAnnotationWizard({ projectId }: CaptionAnnotationWizardProps) {
  const navigate = useNavigate()
  const [tasks, setTasks] = useState<CaptionTask[]>([])
  const [images, setImages] = useState<Image[]>([])
  const [project, setProject] = useState<Project | null>(null)
  const [currentTaskIndex, setCurrentTaskIndex] = useState(0)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [autoCaptioning, setAutoCaptioning] = useState(false)
  const [caption, setCaption] = useState('')

  const currentTask = tasks[currentTaskIndex]
  const totalTasks = tasks.length

  useEffect(() => {
    loadData()
  }, [projectId])

  useEffect(() => {
    if (currentTask) {
      setCaption(currentTask.caption?.String || '')
    }
  }, [currentTask])

  const loadData = async () => {
    setLoading(true)
    try {
      const [tasksResponse, imagesResponse, projectResponse] = await Promise.all([
        getCaptionTasks(projectId),
        getImages(projectId),
        getProject(projectId)
      ])
      setTasks(tasksResponse.data)
      setImages(imagesResponse.data)
      setProject(projectResponse.data)

      // Find first task that needs attention (resume functionality)
      // Priority: auto_generated > pending > completed for review
      let firstNeedsAttention = tasksResponse.data.findIndex(t =>
        t.status === 'auto_generated' && !t.skipped
      )
      
      if (firstNeedsAttention === -1) {
        firstNeedsAttention = tasksResponse.data.findIndex(t =>
          t.status === 'pending' && !t.skipped
        )
      }
      
      if (firstNeedsAttention >= 0) {
        setCurrentTaskIndex(firstNeedsAttention)
      } else {
        // If no tasks need attention, start from beginning for review
        setCurrentTaskIndex(0)
      }
    } catch (error) {
      console.error('Error loading data:', error)
    } finally {
      setLoading(false)
    }
  }

  const getImageById = (id: string): Image | undefined => {
    return images.find(img => img.id === id)
  }

  const getImageUrl = (imagePath: string) => {
    return `http://localhost:8080/projects/${projectId}/${imagePath}`
  }

  const handleSave = async () => {
    if (!currentTask || !caption.trim()) return

    setSaving(true)
    try {
      await updateCaptionTask(currentTask.id, {
        caption: { String: caption.trim(), Valid: true },
        status: 'completed',
        skipped: false
      })

      // Update local task state
      const updatedTasks = [...tasks]
      updatedTasks[currentTaskIndex] = {
        ...currentTask,
        caption: { String: caption.trim(), Valid: true },
        status: 'completed'
      }
      setTasks(updatedTasks)

      // Move to next task
      goToNextTask()
    } catch (error) {
      console.error('Error saving caption:', error)
      alert('Failed to save caption')
    } finally {
      setSaving(false)
    }
  }

  const handleSkip = async () => {
    if (!currentTask) return

    setSaving(true)
    try {
      await updateCaptionTask(currentTask.id, {
        skipped: true
      })

      // Update local task state
      const updatedTasks = [...tasks]
      updatedTasks[currentTaskIndex] = {
        ...currentTask,
        skipped: true
      }
      setTasks(updatedTasks)

      goToNextTask()
    } catch (error) {
      console.error('Error skipping task:', error)
      alert('Failed to skip task')
    } finally {
      setSaving(false)
    }
  }

  const handleAutoCaption = async () => {
    if (!currentTask) return

    setAutoCaptioning(true)
    try {
      const response = await autoCaptionTask(currentTask.id)
      if (response.data.error) {
        alert(`Auto-caption failed: ${response.data.error}`)
      } else if (response.data.caption) {
        setCaption(response.data.caption)
      }
    } catch (error) {
      console.error('Error generating auto-caption:', error)
      alert(`Failed to generate caption automatically: ${error}`)
    } finally {
      setAutoCaptioning(false)
    }
  }

  const handleApprove = async () => {
    if (!currentTask) return

    setSaving(true)
    try {
      await approveCaptionTask(currentTask.id)

      // Update local task state
      const updatedTasks = [...tasks]
      updatedTasks[currentTaskIndex] = {
        ...currentTask,
        status: 'completed'
      }
      setTasks(updatedTasks)

      goToNextTask()
    } catch (error) {
      console.error('Error approving caption:', error)
      alert('Failed to approve caption')
    } finally {
      setSaving(false)
    }
  }

  const handleReject = async () => {
    if (!currentTask) return

    setSaving(true)
    try {
      await rejectCaptionTask(currentTask.id)

      // Update local task state
      const updatedTasks = [...tasks]
      updatedTasks[currentTaskIndex] = {
        ...currentTask,
        caption: { String: '', Valid: false },
        status: 'pending'
      }
      setTasks(updatedTasks)
      setCaption('')

      // Don't move to next task - stay here to redo this caption
    } catch (error) {
      console.error('Error rejecting caption:', error)
      alert('Failed to reject caption')
    } finally {
      setSaving(false)
    }
  }

  const goToNextTask = () => {
    // Priority order: auto_generated > pending > any
    let nextTaskIndex = tasks.findIndex((t, idx) =>
      idx > currentTaskIndex && t.status === 'auto_generated' && !t.skipped
    )

    if (nextTaskIndex === -1) {
      nextTaskIndex = tasks.findIndex((t, idx) =>
        idx > currentTaskIndex && t.status === 'pending' && !t.skipped
      )
    }

    if (nextTaskIndex === -1) {
      // Look for any remaining task that's not completed
      nextTaskIndex = tasks.findIndex((t, idx) =>
        idx > currentTaskIndex && t.status !== 'completed' && !t.skipped
      )
    }

    if (nextTaskIndex >= 0) {
      setCurrentTaskIndex(nextTaskIndex)
    } else {
      // All tasks completed
      navigate(`/projects/${projectId}`)
    }
  }

  const goToPreviousTask = () => {
    if (currentTaskIndex > 0) {
      setCurrentTaskIndex(currentTaskIndex - 1)
    }
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center min-h-64">
        <div className="text-white">Loading caption tasks...</div>
      </div>
    )
  }

  if (tasks.length === 0) {
    return (
      <div className="text-center py-8">
        <h3 className="text-xl font-semibold text-white mb-4">No Caption Tasks Available</h3>
        <p className="text-gray-300 mb-4">Generate caption tasks first to start captioning.</p>
        <button
          onClick={() => navigate(`/projects/${projectId}`)}
          className="px-4 py-2 bg-indigo-600 text-white rounded hover:bg-indigo-700"
        >
          Back to Project
        </button>
      </div>
    )
  }

  if (!currentTask) {
    return (
      <div className="text-center py-8">
        <h3 className="text-xl font-semibold text-white mb-4">All Tasks Completed!</h3>
        <p className="text-gray-300 mb-4">You have finished captioning all available images.</p>
        <button
          onClick={() => navigate(`/projects/${projectId}`)}
          className="px-4 py-2 bg-green-600 text-white rounded hover:bg-green-700"
        >
          Back to Project
        </button>
      </div>
    )
  }

  const currentImage = getImageById(currentTask.imageId)

  return (
    <div className="@container p-3">
      <div className="flex flex-col @4xl:flex-row gap-6">
        {/* Sidebar - Task Navigation */}
        <div className="@4xl:w-80 space-y-4">
          {/* Current Task Info */}
          <div className="bg-white dark:bg-gray-800 rounded-lg p-4 border border-gray-200 dark:border-gray-700">
            <div className="flex justify-between items-center mb-2">
              <h3 className="text-lg font-medium text-gray-900 dark:text-white">Current Task</h3>
              <div className="text-gray-600 dark:text-gray-300">
                {currentTaskIndex + 1} of {totalTasks}
              </div>
            </div>
            {currentTask && (
              <div className="space-y-2 text-sm">
                <div className="text-gray-700 dark:text-gray-300">
                  Task ID: <span className="font-mono text-gray-500 dark:text-gray-400">{currentTask.id.substring(0, 12)}...</span>
                </div>
                <div className="text-gray-700 dark:text-gray-300">
                  Image: <span className="text-gray-500 dark:text-gray-400">{currentImage?.path.split('/').pop()}</span>
                </div>
              </div>
            )}
          </div>

          {/* Task Statistics */}
          <div className="bg-white dark:bg-gray-800 rounded-lg p-4 border border-gray-200 dark:border-gray-700">
            <h3 className="text-lg font-medium text-gray-900 dark:text-white mb-3">Progress</h3>
            <div className="space-y-3">
              {(() => {
                const completedCount = tasks.filter(t => t.status === 'completed' && !t.skipped).length
                const autoGeneratedCount = tasks.filter(t => t.status === 'auto_generated' && !t.skipped).length
                const skippedCount = tasks.filter(t => t.skipped).length
                const pendingCount = tasks.filter(t => t.status === 'pending' && !t.skipped).length
                
                return (
                  <>
                    <div className="flex justify-between text-sm">
                      <span className="text-gray-600 dark:text-gray-400">Completed</span>
                      <span className="text-green-600 dark:text-green-400 font-medium">{completedCount}</span>
                    </div>
                    <div className="flex justify-between text-sm">
                      <span className="text-gray-600 dark:text-gray-400">Auto Generated</span>
                      <span className="text-blue-600 dark:text-blue-400 font-medium">{autoGeneratedCount}</span>
                    </div>
                    <div className="flex justify-between text-sm">
                      <span className="text-gray-600 dark:text-gray-400">Skipped</span>
                      <span className="text-yellow-600 dark:text-yellow-400 font-medium">{skippedCount}</span>
                    </div>
                    <div className="flex justify-between text-sm">
                      <span className="text-gray-600 dark:text-gray-400">Pending</span>
                      <span className="text-purple-600 dark:text-purple-400 font-medium">{pendingCount}</span>
                    </div>
                    <div className="mt-3">
                      <div className="flex justify-between text-xs text-gray-600 dark:text-gray-400 mb-1">
                        <span>Progress</span>
                        <span>{Math.round((completedCount / totalTasks) * 100)}%</span>
                      </div>
                      <div className="w-full bg-gray-200 dark:bg-gray-700 rounded-full h-2">
                        <div
                          className="bg-green-500 h-2 rounded-full transition-all duration-300"
                          style={{ width: `${(completedCount / totalTasks) * 100}%` }}
                        />
                      </div>
                    </div>
                  </>
                )
              })()}
            </div>
          </div>

          {/* Quick Actions */}
          <div className="bg-white dark:bg-gray-800 rounded-lg p-4 border border-gray-200 dark:border-gray-700">
            <h3 className="text-lg font-medium text-gray-900 dark:text-white mb-3">Quick Actions</h3>
            <div className="space-y-2">
              <button
                onClick={() => {
                  // Find next auto_generated task first
                  let nextTask = tasks.findIndex((t, idx) =>
                    idx > currentTaskIndex && t.status === 'auto_generated' && !t.skipped
                  )
                  if (nextTask === -1) {
                    // Then find next pending task
                    nextTask = tasks.findIndex((t, idx) =>
                      idx > currentTaskIndex && t.status === 'pending' && !t.skipped
                    )
                  }
                  if (nextTask >= 0) {
                    setCurrentTaskIndex(nextTask)
                  } else {
                    // Wrap around to first task needing attention
                    const firstNeedsAttention = tasks.findIndex(t =>
                      (t.status === 'auto_generated' || t.status === 'pending') && !t.skipped
                    )
                    if (firstNeedsAttention >= 0) {
                      setCurrentTaskIndex(firstNeedsAttention)
                    }
                  }
                }}
                className="w-full px-3 py-2 text-sm bg-blue-600 text-white rounded-lg hover:bg-blue-700 transition-colors"
              >
                Jump to Next Incomplete
              </button>

              <button
                onClick={() => {
                  // First look for auto_generated tasks
                  let firstNeedsAttention = tasks.findIndex(t =>
                    t.status === 'auto_generated' && !t.skipped
                  )
                  if (firstNeedsAttention === -1) {
                    // Then look for pending tasks
                    firstNeedsAttention = tasks.findIndex(t =>
                      t.status === 'pending' && !t.skipped
                    )
                  }
                  if (firstNeedsAttention >= 0) {
                    setCurrentTaskIndex(firstNeedsAttention)
                  } else {
                    setCurrentTaskIndex(0)
                  }
                }}
                className="w-full px-3 py-2 text-sm bg-gray-600 text-white rounded-lg hover:bg-gray-700 transition-colors"
              >
                Jump to First Incomplete
              </button>
            </div>
          </div>

          {/* Task List */}
          <div className="bg-white dark:bg-gray-800 rounded-lg p-4 border border-gray-200 dark:border-gray-700">
            <h3 className="text-lg font-medium text-gray-900 dark:text-white mb-3">All Tasks</h3>
            <div className="space-y-1 max-h-64 overflow-y-auto">
              {tasks.map((task, index) => {
                const isCompleted = task.status === 'completed' && !task.skipped
                const isAutoGenerated = task.status === 'auto_generated' && !task.skipped
                const isSkipped = task.skipped
                const isCurrent = index === currentTaskIndex

                return (
                  <div
                    key={task.id}
                    className={`flex items-center justify-between p-2 rounded-lg cursor-pointer transition-colors ${isCurrent ? 'bg-blue-600 text-white' :
                      isCompleted ? 'bg-green-50 dark:bg-green-900/20 hover:bg-green-100 dark:hover:bg-green-900/30' :
                        isAutoGenerated ? 'bg-blue-50 dark:bg-blue-900/20 hover:bg-blue-100 dark:hover:bg-blue-900/30' :
                        isSkipped ? 'bg-yellow-50 dark:bg-yellow-900/20 hover:bg-yellow-100 dark:hover:bg-yellow-900/30' :
                          'bg-gray-50 dark:bg-gray-700/50 hover:bg-gray-100 dark:hover:bg-gray-600'
                      }`}
                    onClick={() => setCurrentTaskIndex(index)}
                  >
                    <div className="flex items-center space-x-2">
                      <div className={`text-sm font-mono ${isCurrent ? 'text-white' : 'text-gray-600 dark:text-gray-300'}`}>
                        #{index + 1}
                      </div>
                      <div className={`w-2 h-2 rounded-full ${
                        isCompleted ? 'bg-green-500' :
                        isAutoGenerated ? 'bg-blue-500' :
                        isSkipped ? 'bg-yellow-500' :
                        isCurrent ? 'bg-white' :
                        'bg-gray-400'
                      }`} />
                    </div>

                    <div className={`text-xs px-2 py-1 rounded-full ${
                      isCompleted ? 'bg-green-600 text-white' :
                      isAutoGenerated ? 'bg-blue-600 text-white' :
                      isSkipped ? 'bg-yellow-600 text-white' :
                      isCurrent ? 'bg-white text-blue-600' :
                      'bg-gray-500 text-white'
                    }`}>
                      {isCompleted ? 'Done' : 
                       isAutoGenerated ? 'Review' :
                       isSkipped ? 'Skip' : 'Todo'}
                    </div>
                  </div>
                )
              })}
            </div>
          </div>
        </div>

        {/* Main Content */}
        <div className="flex-1 space-y-6">
          {/* Navigation */}
          <div className="flex justify-between">
            <button
              onClick={goToPreviousTask}
              disabled={currentTaskIndex === 0}
              className="px-4 py-2 bg-gray-600 text-white rounded-lg hover:bg-gray-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
            >
              Previous
            </button>
            <button
              onClick={() => navigate(`/projects/${projectId}`)}
              className="px-4 py-2 bg-gray-600 text-white rounded-lg hover:bg-gray-700 transition-colors"
            >
              Back to Project
            </button>
          </div>

          {/* Current Image */}
          <div className="bg-white dark:bg-gray-800 rounded-lg p-6 border border-gray-200 dark:border-gray-700">
            <h3 className="text-lg font-medium text-gray-900 dark:text-white mb-4">Image to Caption</h3>
            {currentImage ? (
              <div className="space-y-4">
                <div className="flex justify-center">
                  <img
                    src={getImageUrl(currentImage.path)}
                    alt="Image to caption"
                    className="max-w-full max-h-96 object-contain bg-gray-100 dark:bg-gray-900 rounded-lg"
                  />
                </div>
                <div className="text-center">
                  <p className="text-sm text-gray-500 dark:text-gray-400">{currentImage.path.split('/').pop()}</p>
                  <p className="text-xs text-gray-400 dark:text-gray-500 font-mono">{currentImage.pHash.substring(0, 12)}...</p>
                </div>
              </div>
            ) : (
              <div className="w-full h-80 bg-gray-100 dark:bg-gray-900 rounded-lg flex items-center justify-center">
                <span className="text-gray-500 dark:text-gray-400">Image not found</span>
              </div>
            )}
          </div>

          {/* Caption Input */}
          <div className="bg-gray-700 rounded-lg p-4">
            <h3 className="text-lg font-semibold text-white mb-4">Image Caption</h3>
            
            {/* Prompt Buttons */}
            {project?.promptButtons && project.promptButtons.length > 0 && (
              <div className="mb-4">
                <p className="text-sm text-gray-300 mb-2">Quick prompts:</p>
                <div className="flex flex-wrap gap-2">
                  {project.promptButtons.map((button, index) => (
                    <button
                      key={index}
                      onClick={() => setCaption(button)}
                      className="px-3 py-1 text-sm bg-indigo-600 text-white rounded-lg hover:bg-indigo-700 transition-colors"
                    >
                      {button}
                    </button>
                  ))}
                </div>
              </div>
            )}
            
            <textarea
              value={caption}
              onChange={(e) => setCaption(e.target.value)}
              placeholder="Describe what you see in this image..."
              className="w-full p-3 bg-gray-800 border border-gray-600 rounded text-white placeholder-gray-400 resize-none focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500"
              autoFocus
              rows={10}
              
            />
          </div>

          {/* Action Buttons */}
          <div className="flex justify-end space-x-3">
            <button
              onClick={handleSkip}
              disabled={saving || autoCaptioning}
              className="px-6 py-2 bg-yellow-600 text-white rounded hover:bg-yellow-700 disabled:opacity-50"
            >
              Skip Image
            </button>
            {project?.captionApi && (
              <button
                onClick={handleAutoCaption}
                disabled={saving || autoCaptioning}
                className="px-6 py-2 bg-green-600 text-white rounded hover:bg-green-700 disabled:opacity-50"
              >
                {autoCaptioning ? 'Generating...' : 'Auto Caption'}
              </button>
            )}
            
            {/* Review buttons for auto-generated captions */}
            {currentTask?.status === 'auto_generated' && (
              <>
                <button
                  onClick={handleReject}
                  disabled={saving}
                  className="px-6 py-2 bg-red-600 text-white rounded hover:bg-red-700 disabled:opacity-50"
                >
                  {saving ? 'Rejecting...' : 'Reject'}
                </button>
                <button
                  onClick={handleApprove}
                  disabled={saving}
                  className="px-6 py-2 bg-green-600 text-white rounded hover:bg-green-700 disabled:opacity-50"
                >
                  {saving ? 'Approving...' : 'Approve'}
                </button>
              </>
            )}
            
            <button
              onClick={handleSave}
              disabled={saving || autoCaptioning || !caption.trim()}
              className="px-6 py-2 bg-indigo-600 text-white rounded hover:bg-indigo-700 disabled:opacity-50"
            >
              {saving ? 'Saving...' : 'Save & Next'}
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}