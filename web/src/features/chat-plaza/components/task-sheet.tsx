/*
Copyright (C) 2023-2026 1528960014

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@kejiapi.com
*/
import { useQuery } from '@tanstack/react-query'
import dayjs from 'dayjs'
import { History } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Sheet, SheetContent, SheetTitle } from '@/components/ui/sheet'
import { cn } from '@/lib/utils'

import { fetchPlazaTaskLogs, type PlazaTaskLog } from '../lib/api'

const TASK_STATUS_TONES: Record<string, string> = {
  SUCCESS: 'bg-emerald-500/15 text-emerald-300 ring-emerald-400/30',
  IN_PROGRESS: 'bg-cyan-500/15 text-cyan-300 ring-cyan-400/30',
  SUBMITTED: 'bg-cyan-500/15 text-cyan-300 ring-cyan-400/30',
  FAILURE: 'bg-rose-500/15 text-rose-300 ring-rose-400/30',
}

function taskTime(task: PlazaTaskLog): dayjs.Dayjs {
  const seconds = task.created_at ?? task.finish_time ?? task.submit_time
  if (typeof seconds === 'number' && seconds > 0) {
    // TaskLog timestamps are in seconds.
    return dayjs(seconds * 1000)
  }
  return dayjs()
}

function TaskRow({ task }: { task: PlazaTaskLog }) {
  const model =
    task.properties?.upstream_model_name ||
    task.properties?.origin_model_name ||
    task.platform ||
    '—'
  const prompt = task.properties?.input || task.action || ''
  const tone =
    TASK_STATUS_TONES[task.status] ?? 'bg-white/[0.06] text-gray-400 ring-white/10'

  return (
    <div
      key={`${task.task_id}-${task.id}`}
      className='rounded-xl border border-white/[0.07] bg-white/[0.03] p-3'
    >
      <div className='flex items-center gap-2'>
        <span className='min-w-0 flex-1 truncate text-sm font-medium text-white'>
          {model}
        </span>
        <span
          className={cn(
            'shrink-0 rounded-md px-1.5 py-0.5 text-[10px] font-medium ring-1',
            tone
          )}
        >
          {task.status}
        </span>
      </div>
      {prompt ? (
        <p className='text-gray-400 mt-1.5 line-clamp-2 text-xs leading-5'>
          {prompt}
        </p>
      ) : null}
      <div className='mt-1.5 flex items-center justify-between text-[11px] text-gray-500'>
        <span className='truncate'>{task.action}</span>
        <span className='shrink-0'>
          {taskTime(task).format('MM-DD HH:mm')}
        </span>
      </div>
    </div>
  )
}

interface TaskSheetProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

/**
 * Right-side sheet listing the user's recent task history
 * (`GET /api/task/self`), mirroring the reference task list drawer.
 */
export function TaskSheet({ open, onOpenChange }: TaskSheetProps) {
  const { t } = useTranslation()
  const { data: tasks, isLoading, isError } = useQuery({
    queryKey: ['chat-plaza', 'task-logs'],
    queryFn: () => fetchPlazaTaskLogs(20),
    enabled: open,
  })

  const renderBody = () => {
    if (isLoading) {
      return (
        <p className='text-gray-500 pt-8 text-center text-xs'>
          {t('Loading')}
        </p>
      )
    }
    if (isError) {
      return (
        <p className='text-gray-500 pt-8 text-center text-xs'>
          {t('Request error occurred')}
        </p>
      )
    }
    if (!tasks || tasks.length === 0) {
      return (
        <p className='text-gray-500 pt-8 text-center text-xs'>
          {t('No tasks yet')}
        </p>
      )
    }
    return (
      <div className='space-y-2'>
        {tasks.map((task) => (
          <TaskRow key={`${task.task_id}-${task.id}`} task={task} />
        ))}
      </div>
    )
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side='right'
        className='bg-[#0d1118] text-gray-200 sm:max-w-md'
      >
        <div className='flex items-center gap-2 px-4 pt-4 pb-2'>
          <History className='text-cyan-300 size-4' />
          <SheetTitle className='text-white text-base'>
            {t('Task List')}
          </SheetTitle>
          <span className='text-gray-500 text-xs'>
            {tasks ? `(${tasks.length})` : ''}
          </span>
        </div>

        <div className='min-h-0 flex-1 overflow-y-auto px-4 pb-4'>
          {renderBody()}
        </div>
      </SheetContent>
    </Sheet>
  )
}
