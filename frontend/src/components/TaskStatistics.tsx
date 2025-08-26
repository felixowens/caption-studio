import { type Task } from '../api'

interface TaskStatisticsProps {
  tasks: Task[]
}

export function TaskStatistics({ tasks }: TaskStatisticsProps) {
  const totalTasks = tasks.length
  const completedTasks = tasks.filter(t => (t.imageBId?.Valid || t.prompt?.Valid) && !t.skipped).length
  const skippedTasks = tasks.filter(t => t.skipped).length
  const pendingTasks = totalTasks - completedTasks - skippedTasks

  const getPercentage = (count: number) => totalTasks > 0 ? Math.round((count / totalTasks) * 100) : 0

  return (
    <div className="@container bg-white dark:bg-gray-800 rounded-lg p-6 border border-gray-200 dark:border-gray-700">
      <div className="flex justify-between items-center mb-6">
        <h3 className="text-xl font-light text-gray-900 dark:text-white">Task Progress</h3>
        {pendingTasks === 0 && totalTasks > 0 && (
          <div className="flex items-center px-3 py-1 bg-green-600 text-white text-sm rounded-full">
            <svg className="w-4 h-4 mr-1" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
            </svg>
            Complete
          </div>
        )}
      </div>

      {/* Progress Bar */}
      <div className="w-full bg-gray-200 dark:bg-gray-700 rounded-full h-4 mb-6 overflow-hidden">
        <div className="h-full flex rounded-full overflow-hidden">
          <div
            className="bg-green-600 transition-all duration-700 ease-out"
            style={{ width: `${getPercentage(completedTasks)}%` }}
          />
          <div
            className="bg-yellow-500 transition-all duration-700 ease-out"
            style={{ width: `${getPercentage(skippedTasks)}%` }}
          />
          <div
            className="bg-gray-400 transition-all duration-700 ease-out"
            style={{ width: `${getPercentage(pendingTasks)}%` }}
          />
        </div>
      </div>

      {/* Statistics Grid */}
      <div className="grid grid-cols-2 @md:grid-cols-4 gap-4 mb-6">
        <div className="text-center p-4 bg-gray-50 dark:bg-gray-700 rounded-lg">
          <div className="text-3xl font-light text-gray-900 dark:text-white mb-1">{totalTasks}</div>
          <div className="text-sm font-medium text-gray-600 dark:text-gray-400">Total</div>
        </div>

        <div className="text-center p-4 bg-green-50 dark:bg-green-900/20 rounded-lg border border-green-200 dark:border-green-800">
          <div className="text-3xl font-light text-green-700 dark:text-green-400 mb-1">{completedTasks}</div>
          <div className="text-sm font-medium text-green-600 dark:text-green-400">Completed</div>
          <div className="text-xs text-green-500 dark:text-green-500">({getPercentage(completedTasks)}%)</div>
        </div>

        <div className="text-center p-4 bg-yellow-50 dark:bg-yellow-900/20 rounded-lg border border-yellow-200 dark:border-yellow-800">
          <div className="text-3xl font-light text-yellow-700 dark:text-yellow-400 mb-1">{skippedTasks}</div>
          <div className="text-sm font-medium text-yellow-600 dark:text-yellow-400">Skipped</div>
          <div className="text-xs text-yellow-500 dark:text-yellow-500">({getPercentage(skippedTasks)}%)</div>
        </div>

        <div className="text-center p-4 bg-gray-50 dark:bg-gray-900/20 rounded-lg border border-gray-200 dark:border-gray-700">
          <div className="text-3xl font-light text-gray-700 dark:text-gray-400 mb-1">{pendingTasks}</div>
          <div className="text-sm font-medium text-gray-600 dark:text-gray-400">Pending</div>
          <div className="text-xs text-gray-500 dark:text-gray-500">({getPercentage(pendingTasks)}%)</div>
        </div>
      </div>

      {/* Status Legend */}
      <div className="flex justify-center items-center space-x-6 text-sm">
        <div className="flex items-center space-x-2">
          <div className="w-3 h-3 bg-green-600 rounded-full"></div>
          <span className="text-gray-700 dark:text-gray-300 font-medium">Completed</span>
        </div>
        <div className="flex items-center space-x-2">
          <div className="w-3 h-3 bg-yellow-500 rounded-full"></div>
          <span className="text-gray-700 dark:text-gray-300 font-medium">Skipped</span>
        </div>
        <div className="flex items-center space-x-2">
          <div className="w-3 h-3 bg-gray-400 rounded-full"></div>
          <span className="text-gray-700 dark:text-gray-300 font-medium">Pending</span>
        </div>
      </div>
    </div>
  )
}