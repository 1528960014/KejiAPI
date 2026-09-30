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
import { LoaderCircle } from 'lucide-react'
import { useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'

import { Markdown } from '@/components/ui/markdown'
import { cn } from '@/lib/utils'

import type { PlazaMessage } from '../types'

interface MessageStreamProps {
  messages: PlazaMessage[]
  modelIcon?: string
  modelName?: string
}

/**
 * Lightweight chat transcript rendered between the stage and the
 * creator input: user bubble (right, cyan tint) and assistant bubble
 * (left, Markdown when complete, raw text while streaming).
 */
export function MessageStream({
  messages,
  modelIcon,
  modelName,
}: MessageStreamProps) {
  const { t } = useTranslation()
  const scrollRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const el = scrollRef.current
    if (el) el.scrollTop = el.scrollHeight
  }, [messages])

  if (messages.length === 0) return null

  return (
    <div
      ref={scrollRef}
      className='mx-auto min-h-0 w-full max-w-[800px] overflow-y-auto px-4 pb-2 [scrollbar-width:thin] [scrollbar-color:rgba(255,255,255,0.15)_transparent]'
      style={{ maxHeight: 'min(30vh, 320px)' }}
    >
      <div className='space-y-3 pb-1'>
        {messages.map((message) =>
          message.role === 'user' ? (
            <div
              key={message.id}
              className='bg-cyan-400/10 border-cyan-400/25 flex justify-end'
            >
              <div className='max-w-[85%] whitespace-pre-wrap rounded-2xl rounded-br-md border px-3.5 py-2.5 text-sm text-white'>
                {message.content}
              </div>
            </div>
          ) : (
            <div key={message.id} className='flex items-start gap-2.5'>
              <div className='flex size-7 shrink-0 items-center justify-center overflow-hidden rounded-lg bg-white/[0.06] ring-1 ring-white/10'>
                {modelIcon ? (
                  <img
                    src={modelIcon}
                    alt=''
                    className='size-7 object-contain'
                  />
                ) : (
                  <span className='text-cyan-300 text-[10px] font-bold'>
                    {(modelName ?? 'AI')
                      .trim()
                      .charAt(0)
                      .toUpperCase()}
                  </span>
                )}
              </div>
              <div
                className={cn(
                  'flex min-w-0 max-w-[85%] flex-col rounded-2xl rounded-bl-md border px-3.5 py-2.5 text-sm',
                  message.status === 'error'
                    ? 'border-rose-400/30 bg-rose-500/[0.06] text-rose-200'
                    : 'border-white/10 bg-white/[0.05] text-gray-200'
                )}
              >
                {message.status === 'streaming' && (
                  <LoaderCircle className='mb-1.5 size-3.5 animate-spin text-cyan-300' />
                )}
                {message.status === 'complete' && message.content ? (
                  <Markdown breaks className='max-w-none text-sm'>
                    {message.content}
                  </Markdown>
                ) : (
                  <span className='whitespace-pre-wrap'>
                    {message.status === 'error'
                      ? `${t('Request error occurred')}: ${message.error ?? ''}`
                      : message.content || ''}
                    {message.status === 'streaming' ? (
                      <span className='ml-0.5 inline-block h-3.5 w-1.5 animate-pulse bg-cyan-300/80 align-middle' />
                    ) : null}
                  </span>
                )}
              </div>
            </div>
          )
        )}
      </div>
    </div>
  )
}
