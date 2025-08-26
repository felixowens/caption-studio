import { useState, useCallback, useRef, useEffect } from 'react'
import { uploadFiles, createProgressEventSource, type ProgressUpdate } from '../api'

interface FileUploadProps {
  projectId: string
  onUploadComplete?: () => void
}

export function FileUpload({ projectId, onUploadComplete }: FileUploadProps) {
  const [isDragOver, setIsDragOver] = useState(false)
  const [isUploading, setIsUploading] = useState(false)
  const [progress, setProgress] = useState<ProgressUpdate | null>(null)
  const [error, setError] = useState<string | null>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)
  const eventSourceRef = useRef<EventSource | null>(null)

  const handleDragOver = useCallback((e: React.DragEvent) => {
    e.preventDefault()
    setIsDragOver(true)
  }, [])

  const handleDragLeave = useCallback((e: React.DragEvent) => {
    e.preventDefault()
    setIsDragOver(false)
  }, [])

  const handleDrop = useCallback((e: React.DragEvent) => {
    e.preventDefault()
    setIsDragOver(false)
    
    const files = e.dataTransfer.files
    if (files.length > 0) {
      handleFiles(files)
    }
  }, [])

  const handleFileSelect = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    const files = e.target.files
    if (files && files.length > 0) {
      handleFiles(files)
    }
  }, [])

  const handleFiles = async (files: FileList) => {
    setError(null)
    setIsUploading(true)
    setProgress(null)

    // Filter for image files only
    const imageFiles = Array.from(files).filter(file => 
      file.type.startsWith('image/')
    )

    if (imageFiles.length === 0) {
      setError('Please select only image files (JPEG, PNG, etc.)')
      setIsUploading(false)
      return
    }

    // Create new FileList with only image files
    const dataTransfer = new DataTransfer()
    imageFiles.forEach(file => dataTransfer.items.add(file))
    const imageFileList = dataTransfer.files

    try {
      // Set up progress tracking
      eventSourceRef.current = createProgressEventSource(projectId)
      eventSourceRef.current.onmessage = (event) => {
        const update: ProgressUpdate = JSON.parse(event.data)
        setProgress(update)
        
        if (update.status === 'completed') {
          setIsUploading(false)
          eventSourceRef.current?.close()
          eventSourceRef.current = null
          onUploadComplete?.()
        } else if (update.status === 'error') {
          setError(update.errorMessage || 'Upload failed')
        }
      }

      eventSourceRef.current.onerror = () => {
        setError('Connection to server lost')
        setIsUploading(false)
        eventSourceRef.current?.close()
        eventSourceRef.current = null
      }

      // Start upload
      await uploadFiles(projectId, imageFileList)
    } catch (err) {
      console.error('Upload error:', err)
      setError('Failed to start upload')
      setIsUploading(false)
      eventSourceRef.current?.close()
      eventSourceRef.current = null
    }
  }

  const handleBrowseClick = () => {
    fileInputRef.current?.click()
  }

  useEffect(() => {
    return () => {
      eventSourceRef.current?.close()
    }
  }, [])

  const progressPercentage = progress ? Math.round((progress.progress / progress.total) * 100) : 0

  return (
    <div className="w-full">
      <div
        className={`border-2 border-dashed rounded-xl p-8 text-center transition-colors ${
          isDragOver
            ? 'border-indigo-400 bg-indigo-50/10'
            : isUploading
            ? 'border-yellow-400 bg-yellow-50/10'
            : 'border-gray-500 hover:border-gray-400'
        }`}
        onDragOver={handleDragOver}
        onDragLeave={handleDragLeave}
        onDrop={handleDrop}
      >
        <input
          ref={fileInputRef}
          type="file"
          multiple
          accept="image/*"
          className="hidden"
          onChange={handleFileSelect}
          disabled={isUploading}
        />

        {isUploading ? (
          <div className="space-y-4">
            <div className="text-yellow-400">
              <svg className="w-12 h-12 mx-auto animate-spin" fill="none" viewBox="0 0 24 24">
                <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"></circle>
                <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
              </svg>
            </div>
            <div className="text-white">
              <p className="text-lg font-medium">Uploading Images...</p>
              {progress && (
                <>
                  <p className="text-sm text-gray-300 mt-2">
                    {progress.status === 'processing' ? `Processing: ${progress.filename}` : progress.status}
                  </p>
                  <div className="w-full bg-gray-700 rounded-full h-2 mt-3">
                    <div
                      className="bg-indigo-600 h-2 rounded-full transition-all duration-300"
                      style={{ width: `${progressPercentage}%` }}
                    ></div>
                  </div>
                  <p className="text-xs text-gray-400 mt-1">
                    {progress.progress} / {progress.total} files ({progressPercentage}%)
                  </p>
                </>
              )}
            </div>
          </div>
        ) : (
          <div className="space-y-4">
            <div className="text-gray-400">
              <svg className="w-12 h-12 mx-auto" fill="none" stroke="currentColor" viewBox="0 0 48 48">
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={2}
                  d="M28 8H12a4 4 0 00-4 4v20m32-12v8m0 0v8a4 4 0 01-4 4H12a4 4 0 01-4-4v-4m32-4l-3.172-3.172a4 4 0 00-5.656 0L28 28M8 32l9.172-9.172a4 4 0 015.656 0L28 28m0 0l4 4m4-24h8m-4-4v8m-12 4h.02"
                />
              </svg>
            </div>
            <div className="text-white">
              <p className="text-lg font-medium">Drop images here</p>
              <p className="text-sm text-gray-300 mt-1">or</p>
              <button
                onClick={handleBrowseClick}
                className="mt-2 px-4 py-2 bg-indigo-600 text-white rounded-md hover:bg-indigo-700 transition-colors focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:ring-offset-2 focus:ring-offset-gray-800"
              >
                Browse Files
              </button>
            </div>
            <p className="text-xs text-gray-500">
              Supports JPEG, PNG and other image formats
            </p>
          </div>
        )}
      </div>

      {error && (
        <div className="mt-4 p-4 bg-red-900/50 border border-red-500 rounded-lg">
          <p className="text-red-200 text-sm">{error}</p>
        </div>
      )}
    </div>
  )
}