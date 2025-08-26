import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { getTasks, getImages, updateTask, getProject, type Task, type Image, type Project } from '../api'
import { TaskStatistics } from './TaskStatistics'

interface AnnotationWizardProps {
  projectId: string
}

export function AnnotationWizard({ projectId }: AnnotationWizardProps) {
  const navigate = useNavigate()
  const [tasks, setTasks] = useState<Task[]>([])
  const [images, setImages] = useState<Image[]>([])
  const [project, setProject] = useState<Project | null>(null)
  const [currentTaskIndex, setCurrentTaskIndex] = useState(0)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [prompt, setPrompt] = useState('')
  const [selectedImageBId, setSelectedImageBId] = useState<string>('')
  const [showAllImages, setShowAllImages] = useState(false)

  const currentTask = tasks[currentTaskIndex]
  const totalTasks = tasks.length

  useEffect(() => {
    loadData()
  }, [projectId])

  useEffect(() => {
    if (currentTask) {
      setPrompt(currentTask.prompt?.String || '')
      setSelectedImageBId(currentTask.imageBId?.String || '')
    }
  }, [currentTask])

  const loadData = async () => {
    setLoading(true)
    try {
      const [tasksResponse, imagesResponse, projectResponse] = await Promise.all([
        getTasks(projectId),
        getImages(projectId),
        getProject(projectId)
      ])
      setTasks(tasksResponse.data)
      setImages(imagesResponse.data)
      setProject(projectResponse.data)

      // Find first incomplete task (resume functionality)
      const firstIncomplete = tasksResponse.data.findIndex(t =>
        !(t.imageBId?.Valid || t.prompt?.Valid) && !t.skipped
      )
      if (firstIncomplete >= 0) {
        setCurrentTaskIndex(firstIncomplete)
      } else {
        // If no incomplete tasks, start from beginning for review
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

  // Calculate Hamming distance between two pHash strings
  // goimagehash format is typically "p:" followed by 16 hex characters for 64-bit hash
  const calculatePHashDistance = (hash1: string, hash2: string): number => {
    if (!hash1 || !hash2) {
      return Number.MAX_SAFE_INTEGER
    }

    try {
      // Parse goimagehash format: "p:xxxxxxxxxxxx"
      let cleanHash1 = hash1
      let cleanHash2 = hash2

      // Extract hex part after "p:" prefix
      if (hash1.startsWith('p:')) {
        cleanHash1 = hash1.substring(2)
      }
      if (hash2.startsWith('p:')) {
        cleanHash2 = hash2.substring(2)
      }

      // Validate hex strings
      if (!cleanHash1 || !cleanHash2 || cleanHash1.length !== cleanHash2.length) {
        return Number.MAX_SAFE_INTEGER
      }

      // Ensure they are valid hex
      if (!/^[0-9a-fA-F]+$/.test(cleanHash1) || !/^[0-9a-fA-F]+$/.test(cleanHash2)) {
        return Number.MAX_SAFE_INTEGER
      }

      // For very long hashes, process in chunks to avoid BigInt overflow
      let distance = 0
      const chunkSize = 15 // Process 15 hex chars at a time (60 bits, safe for BigInt)
      
      for (let i = 0; i < cleanHash1.length; i += chunkSize) {
        const chunk1 = cleanHash1.substring(i, i + chunkSize)
        const chunk2 = cleanHash2.substring(i, i + chunkSize)
        
        if (chunk1.length !== chunk2.length) break
        
        const bigint1 = BigInt('0x' + chunk1)
        const bigint2 = BigInt('0x' + chunk2)
        
        // XOR to get differing bits
        let xor = bigint1 ^ bigint2
        
        // Count set bits in this chunk
        while (xor > 0n) {
          distance++
          xor = xor & (xor - 1n) // Remove lowest set bit
        }
      }
      
      return distance
    } catch (error) {
      // Fallback: simple character comparison for debugging
      console.warn('Error calculating pHash distance:', error)
      let distance = 0
      const minLength = Math.min(hash1.length, hash2.length)
      for (let i = 0; i < minLength; i++) {
        if (hash1[i] !== hash2[i]) {
          distance++
        }
      }
      return distance + Math.abs(hash1.length - hash2.length)
    }
  }

  // Sort images by pHash similarity to a reference image
  const sortImagesByPHashSimilarity = (referenceImage: Image, images: Image[]): Image[] => {
    if (!referenceImage || !referenceImage.pHash) {
      return images // Return unsorted if no reference hash
    }

    return [...images].sort((a, b) => {
      const distanceA = calculatePHashDistance(referenceImage.pHash, a.pHash)
      const distanceB = calculatePHashDistance(referenceImage.pHash, b.pHash)
      return distanceA - distanceB // Ascending order (most similar first)
    })
  }

  const handleSave = async () => {
    if (!currentTask || (!selectedImageBId && !prompt.trim())) return

    setSaving(true)
    try {
      await updateTask(currentTask.id, {
        imageBId: selectedImageBId ? { String: selectedImageBId, Valid: true } : null,
        prompt: prompt.trim() ? { String: prompt.trim(), Valid: true } : null,
        skipped: false
      })

      // Update local task state
      const updatedTasks = [...tasks]
      updatedTasks[currentTaskIndex] = {
        ...currentTask,
        imageBId: { String: selectedImageBId, Valid: !!selectedImageBId },
        prompt: { String: prompt.trim(), Valid: !!prompt.trim() }
      }
      setTasks(updatedTasks)

      // Move to next task
      goToNextTask()
    } catch (error) {
      console.error('Error saving task:', error)
      alert('Failed to save annotation')
    } finally {
      setSaving(false)
    }
  }

  const handleSkip = async () => {
    if (!currentTask) return

    setSaving(true)
    try {
      await updateTask(currentTask.id, {
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

  const goToNextTask = () => {
    const nextIncompleteIndex = tasks.findIndex((t, idx) =>
      idx > currentTaskIndex && !(t.imageBId?.Valid || t.prompt?.Valid) && !t.skipped
    )

    if (nextIncompleteIndex >= 0) {
      setCurrentTaskIndex(nextIncompleteIndex)
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
        <div className="text-white">Loading annotation tasks...</div>
      </div>
    )
  }

  if (tasks.length === 0) {
    return (
      <div className="text-center py-8">
        <h3 className="text-xl font-semibold text-white mb-4">No Tasks Available</h3>
        <p className="text-gray-300 mb-4">Generate tasks first to start annotating.</p>
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
        <p className="text-gray-300 mb-4">You have finished annotating all available tasks.</p>
        <button
          onClick={() => navigate(`/projects/${projectId}`)}
          className="px-4 py-2 bg-green-600 text-white rounded hover:bg-green-700"
        >
          Back to Project
        </button>
      </div>
    )
  }

  const imageA = getImageById(currentTask.imageAId)
  const candidateImages = currentTask.candidateBIds
    ?.map(id => getImageById(id))
    .filter(Boolean) as Image[] || []

  return (
    <div className="@container p-3">
      <div className="flex flex-col @4xl:flex-row gap-6">
        {/* Sidebar - Task Navigation */}
        <div className="@4xl:w-80 space-y-4">
          {/* Task Statistics */}
          <TaskStatistics tasks={tasks} />

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
                  Candidates: <span className="text-gray-500 dark:text-gray-400">{candidateImages.length}</span>
                </div>
              </div>
            )}
          </div>

          {/* Quick Actions */}
          <div className="bg-white dark:bg-gray-800 rounded-lg p-4 border border-gray-200 dark:border-gray-700">
            <h3 className="text-lg font-medium text-gray-900 dark:text-white mb-3">Quick Actions</h3>
            <div className="space-y-2">
              <button
                onClick={() => {
                  const nextIncomplete = tasks.findIndex((t, idx) =>
                    idx > currentTaskIndex && !(t.imageBId?.Valid || t.prompt?.Valid) && !t.skipped
                  )
                  if (nextIncomplete >= 0) {
                    setCurrentTaskIndex(nextIncomplete)
                  } else {
                    // Wrap around to first incomplete
                    const firstIncomplete = tasks.findIndex(t =>
                      !(t.imageBId?.Valid || t.prompt?.Valid) && !t.skipped
                    )
                    if (firstIncomplete >= 0) {
                      setCurrentTaskIndex(firstIncomplete)
                    }
                  }
                }}
                className="w-full px-3 py-2 text-sm bg-blue-600 text-white rounded-lg hover:bg-blue-700 transition-colors"
              >
                Jump to Next Incomplete
              </button>

              <button
                onClick={() => {
                  const firstIncomplete = tasks.findIndex(t =>
                    !(t.imageBId?.Valid || t.prompt?.Valid) && !t.skipped
                  )
                  if (firstIncomplete >= 0) {
                    setCurrentTaskIndex(firstIncomplete)
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
                const isCompleted = (task.imageBId?.Valid || task.prompt?.Valid) && !task.skipped
                const isSkipped = task.skipped
                const isCurrent = index === currentTaskIndex

                return (
                  <div
                    key={task.id}
                    className={`flex items-center justify-between p-2 rounded-lg cursor-pointer transition-colors ${isCurrent ? 'bg-blue-600 text-white' :
                      isCompleted ? 'bg-green-50 dark:bg-green-900/20 hover:bg-green-100 dark:hover:bg-green-900/30' :
                        isSkipped ? 'bg-yellow-50 dark:bg-yellow-900/20 hover:bg-yellow-100 dark:hover:bg-yellow-900/30' :
                          'bg-gray-50 dark:bg-gray-700/50 hover:bg-gray-100 dark:hover:bg-gray-600'
                      }`}
                    onClick={() => setCurrentTaskIndex(index)}
                  >
                    <div className="flex items-center space-x-2">
                      <div className={`text-sm font-mono ${isCurrent ? 'text-white' : 'text-gray-600 dark:text-gray-300'}`}>
                        #{index + 1}
                      </div>
                      <div className={`w-2 h-2 rounded-full ${isCompleted ? 'bg-green-500' :
                        isSkipped ? 'bg-yellow-500' :
                          isCurrent ? 'bg-white' :
                            'bg-gray-400'
                        }`} />
                    </div>

                    <div className={`text-xs px-2 py-1 rounded-full ${isCompleted ? 'bg-green-600 text-white' :
                      isSkipped ? 'bg-yellow-600 text-white' :
                        isCurrent ? 'bg-white text-blue-600' :
                          'bg-gray-500 text-white'
                      }`}>
                      {isCompleted ? 'Done' : isSkipped ? 'Skip' : 'Todo'}
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

          <div className="grid grid-cols-1 @3xl:grid-cols-2 gap-3">
            {/* Source Image (A) */}
            <div className="bg-white dark:bg-gray-800 rounded-lg p-4 border border-gray-200 dark:border-gray-700">
              <h3 className="text-lg font-medium text-gray-900 dark:text-white mb-4">Source Image (A)</h3>
              {imageA ? (
                <div className="space-y-2">
                  <img
                    src={getImageUrl(imageA.path)}
                    alt="Source image"
                    className="w-full h-1/4 object-contain bg-gray-100 dark:bg-gray-900 rounded-lg"
                  />
                  <p className="text-xs text-gray-500 dark:text-gray-400 text-center">{imageA.path.split('/').pop()}</p>
                </div>
              ) : (
                <div className="w-full h-80 bg-gray-100 dark:bg-gray-900 rounded-lg flex items-center justify-center">
                  <span className="text-gray-500 dark:text-gray-400">Image not found</span>
                </div>
              )}
            </div>

            {/* Selected Target Image (B) */}
            <div className="bg-white dark:bg-gray-800 rounded-lg p-4 border border-gray-200 dark:border-gray-700">
              <h3 className="text-lg font-medium text-gray-900 dark:text-white mb-4">Selected Target Image (B)</h3>
              {selectedImageBId ? (
                <div className="space-y-2">
                  <img
                    src={getImageUrl(getImageById(selectedImageBId)?.path || '')}
                    alt="Selected target image"
                    className="w-full h-1/4 object-contain bg-gray-100 dark:bg-gray-900 rounded-lg"
                  />
                  <p className="text-xs text-gray-500 dark:text-gray-400 text-center">
                    {getImageById(selectedImageBId)?.path.split('/').pop()}
                  </p>
                  <button
                    onClick={() => setSelectedImageBId('')}
                    className="w-full px-3 py-2 text-sm bg-red-600 text-white rounded-lg hover:bg-red-700 transition-colors"
                  >
                    Clear Selection
                  </button>
                </div>
              ) : (
                <div className="w-full h-80 bg-gray-100 dark:bg-gray-900 rounded-lg flex items-center justify-center">
                  <span className="text-gray-500 dark:text-gray-400">No image selected</span>
                </div>
              )}
            </div>

            {/* Image Selection */}
            <div className="bg-white dark:bg-gray-800 rounded-lg p-4 border border-gray-200 dark:border-gray-700 @3xl:col-span-2">
              <div className="flex justify-between items-center mb-4">
                <h3 className="text-lg font-medium text-gray-900 dark:text-white">
                  Choose Target Image (B)
                </h3>
                {candidateImages.length > 0 && (
                  <div className="flex items-center space-x-2">
                    <button
                      onClick={() => setShowAllImages(false)}
                      className={`px-3 py-1 text-sm rounded-lg transition-colors ${!showAllImages
                        ? 'bg-blue-600 text-white'
                        : 'bg-gray-200 dark:bg-gray-700 text-gray-700 dark:text-gray-300 hover:bg-gray-300 dark:hover:bg-gray-600'
                        }`}
                    >
                      Candidates ({candidateImages.length})
                    </button>
                    <button
                      onClick={() => setShowAllImages(true)}
                      className={`px-3 py-1 text-sm rounded-lg transition-colors ${showAllImages
                        ? 'bg-blue-600 text-white'
                        : 'bg-gray-200 dark:bg-gray-700 text-gray-700 dark:text-gray-300 hover:bg-gray-300 dark:hover:bg-gray-600'
                        }`}
                    >
                      All Images (by similarity)
                    </button>
                  </div>
                )}
              </div>

              {candidateImages.length > 0 && !showAllImages ? (
                <div className="space-y-2 max-h-80 overflow-y-auto">
                  {candidateImages.map((image) => (
                    <div
                      key={image.id}
                      className={`cursor-pointer border-2 rounded-lg p-3 transition-all ${selectedImageBId === image.id
                        ? 'border-blue-500 bg-blue-50 dark:bg-blue-900/20'
                        : 'border-gray-300 dark:border-gray-600 hover:border-gray-400 dark:hover:border-gray-500'
                        }`}
                      onClick={() => setSelectedImageBId(image.id)}
                    >
                      <div className="flex items-center space-x-3">
                        <img
                          src={getImageUrl(image.path)}
                          alt="Candidate image"
                          className="w-20 h-20 object-contain bg-gray-100 dark:bg-gray-900 rounded-lg"
                        />
                        <div className="flex-1 min-w-0">
                          <p className="text-sm text-gray-900 dark:text-white truncate">{image.path.split('/').pop()}</p>
                          <div className="flex items-center justify-between">
                            <p className="text-xs text-gray-500 dark:text-gray-400 font-mono">{image.pHash.substring(0, 12)}...</p>
                            {imageA && (
                              <p className="text-xs text-gray-500 dark:text-gray-400">
                                similarity: {Math.max(0, 64 - calculatePHashDistance(imageA.pHash, image.pHash))}/64
                              </p>
                            )}
                          </div>
                        </div>
                      </div>
                    </div>
                  ))}
                </div>
              ) : (
                <div className="space-y-2 max-h-80 overflow-y-auto">
                  {sortImagesByPHashSimilarity(
                    imageA!, 
                    images.filter(img => img.id !== currentTask.imageAId)
                  ).map((image) => (
                    <div
                      key={image.id}
                      className={`cursor-pointer border-2 rounded-lg p-3 transition-all ${selectedImageBId === image.id
                        ? 'border-blue-500 bg-blue-50 dark:bg-blue-900/20'
                        : 'border-gray-300 dark:border-gray-600 hover:border-gray-400 dark:hover:border-gray-500'
                        }`}
                      onClick={() => setSelectedImageBId(image.id)}
                    >
                      <div className="flex items-center space-x-3">
                        <img
                          src={getImageUrl(image.path)}
                          alt="Project image"
                          className="w-20 h-20 object-contain bg-gray-100 dark:bg-gray-900 rounded-lg"
                        />
                        <div className="flex-1 min-w-0">
                          <p className="text-sm text-gray-900 dark:text-white truncate">{image.path.split('/').pop()}</p>
                          <div className="flex items-center justify-between">
                            <p className="text-xs text-gray-500 dark:text-gray-400 font-mono">{image.pHash.substring(0, 12)}...</p>
                            {imageA && (
                              <p className="text-xs text-gray-500 dark:text-gray-400">
                                similarity: {Math.max(0, 64 - calculatePHashDistance(imageA.pHash, image.pHash))}/64
                              </p>
                            )}
                          </div>
                        </div>
                      </div>
                    </div>
                  ))}
                  {images.filter(img => img.id !== currentTask.imageAId).length === 0 && (
                    <div className="text-center py-8 text-gray-400">
                      <p>No other images available</p>
                      <p className="text-sm">Upload more images to select from</p>
                    </div>
                  )}
                </div>
              )}
            </div>
          </div>

          {/* Prompt Input */}
          <div className="bg-gray-700 rounded-lg p-4">
            <h3 className="text-lg font-semibold text-white mb-4">Edit Description</h3>
            
            {/* Prompt Buttons */}
            {project?.promptButtons && project.promptButtons.length > 0 && (
              <div className="mb-4">
                <p className="text-sm text-gray-300 mb-2">Quick prompts:</p>
                <div className="flex flex-wrap gap-2">
                  {project.promptButtons.map((button, index) => (
                    <button
                      key={index}
                      onClick={() => setPrompt(button)}
                      className="px-3 py-1 text-sm bg-indigo-600 text-white rounded-lg hover:bg-indigo-700 transition-colors"
                    >
                      {button}
                    </button>
                  ))}
                </div>
              </div>
            )}
            
            <textarea
              value={prompt}
              onChange={(e) => setPrompt(e.target.value)}
              placeholder="Describe what changes were made from image A to image B..."
              className="w-full h-24 p-3 bg-gray-800 border border-gray-600 rounded text-white placeholder-gray-400 resize-none focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500"
              autoFocus
            />
          </div>

          {/* Action Buttons */}
          <div className="flex justify-end space-x-3">
            <button
              onClick={handleSkip}
              disabled={saving}
              className="px-6 py-2 bg-yellow-600 text-white rounded hover:bg-yellow-700 disabled:opacity-50"
            >
              Skip Task
            </button>
            <button
              onClick={handleSave}
              disabled={saving || (!selectedImageBId && !prompt.trim())}
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