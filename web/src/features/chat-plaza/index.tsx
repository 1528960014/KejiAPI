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
import { nanoid } from 'nanoid'
import { useCallback, useMemo, useRef, useState } from 'react'

import { useStreamRequest } from '@/features/playground/hooks'
import type { ChatCompletionRequest } from '@/features/playground/types'

import { CreatorInput } from './components/creator-input'
import { MessageStream } from './components/message-stream'
import { ModelListPanel } from './components/model-list-panel'
import { Stage } from './components/stage'
import { TaskSheet } from './components/task-sheet'
import { fetchPlazaModels } from './lib/api'
import { estimateCreditsPerSecond } from './lib/estimate'
import {
  DEFAULT_PLAZA_SETTINGS,
  type PlazaCategory,
  type PlazaMessage,
  type PlazaModel,
  type PlazaRefFile,
  type PlazaRefSelection,
  type PlazaSettings,
} from './types'

/** Fallback chat model when no plaza model is selected yet. */
const DEFAULT_CHAT_MODEL = 'gpt-4o'
/** How many past turns are forwarded to the chat endpoint. */
const MAX_CONTEXT_TURNS = 10

/**
 * Merge a streaming chunk the same way the playground does: full-text
 * replacements keep the tail, otherwise append.
 */
function mergeStreamChunk(current: string, next: string): string {
  if (!current || !next.startsWith(current)) return current + next
  return next
}

/**
 * 对话广场 (Chat Plaza): a two-column, dark-styled generator workbench.
 * Left: model list with category tabs / search / balance card.
 * Right: stage with the selected model, a lightweight streaming
 * transcript, and the creator input bar.
 */
export function ChatPlaza() {
  const [category, setCategory] = useState<PlazaCategory>('all')
  const [search, setSearch] = useState('')
  const [selectedModel, setSelectedModel] = useState<PlazaModel | null>(null)
  const [settings, setSettings] = useState<PlazaSettings>(
    DEFAULT_PLAZA_SETTINGS
  )
  const [prompt, setPrompt] = useState('')
  const [refs, setRefs] = useState<PlazaRefSelection>({})
  const [taskSheetOpen, setTaskSheetOpen] = useState(false)
  const [messages, setMessages] = useState<PlazaMessage[]>([])
  const [isGenerating, setIsGenerating] = useState(false)
  const generationRef = useRef(0)

  const { data: models, isLoading: isModelsLoading } = useQuery({
    queryKey: ['chat-plaza', 'models'],
    queryFn: fetchPlazaModels,
  })

  const { sendStreamRequest, stopStream, isStreaming } = useStreamRequest()

  // Keep the first model selected once the list arrives.
  const firstModelName = models?.[0]?.name
  const effectiveSelected = useMemo(() => {
    if (selectedModel && models?.some((m) => m.name === selectedModel.name)) {
      return selectedModel
    }
    return models?.find((m) => m.name === firstModelName) ?? null
  }, [selectedModel, models, firstModelName])

  const estimatedCredits = useMemo(
    () => estimateCreditsPerSecond(effectiveSelected),
    [effectiveSelected]
  )

  const handleRefPick = useCallback((slot: 'image' | 'video' | 'audio', file: PlazaRefFile) => {
    setRefs((prev) => ({ ...prev, [slot]: file }))
  }, [])

  const handleRefClear = useCallback((slot: 'image' | 'video' | 'audio') => {
    setRefs((prev) => {
      const current = prev[slot]
      if (current?.previewUrl) URL.revokeObjectURL(current.previewUrl)
      return { ...prev, [slot]: null }
    })
  }, [])

  const handleSend = useCallback(() => {
    const text = prompt.trim()
    if (!text || isGenerating) return

    const model = effectiveSelected?.name ?? DEFAULT_CHAT_MODEL
    const history = messages
      .filter((message) => message.status === 'complete' && message.content)
      .slice(-MAX_CONTEXT_TURNS)

    const userMessage: PlazaMessage = {
      id: nanoid(),
      role: 'user',
      content: text,
      status: 'complete',
    }
    const assistantMessage: PlazaMessage = {
      id: nanoid(),
      role: 'assistant',
      content: '',
      status: 'streaming',
    }

    setMessages((prev) => [...prev, userMessage, assistantMessage])
    setIsGenerating(true)
    setPrompt('')

    const generation = generationRef.current + 1
    generationRef.current = generation

    const updateAssistant = (
      updater: (message: PlazaMessage) => PlazaMessage
    ) => {
      setMessages((prev) => {
        if (generationRef.current !== generation) return prev
        const last = prev.at(-1)
        if (!last || last.role !== 'assistant') return prev
        return [...prev.slice(0, -1), updater(last)]
      })
    }

    const payload: ChatCompletionRequest = {
      model,
      stream: true,
      messages: [
        ...history.map((message) => ({
          role: message.role,
          content: message.content,
        })),
        { role: 'user' as const, content: text },
      ],
    }

    sendStreamRequest(
      payload,
      (_type, chunk) => {
        updateAssistant((message) => ({
          ...message,
          content: mergeStreamChunk(message.content, chunk),
        }))
      },
      () => {
        if (generationRef.current !== generation) return
        setIsGenerating(false)
        updateAssistant((message) =>
          message.status === 'streaming'
            ? { ...message, status: 'complete' }
            : message
        )
      },
      (error) => {
        if (generationRef.current !== generation) return
        setIsGenerating(false)
        updateAssistant((message) => ({
          ...message,
          status: 'error',
          error,
        }))
      }
    )
  }, [prompt, isGenerating, messages, effectiveSelected, sendStreamRequest])

  const handleStop = useCallback(() => {
    generationRef.current += 1
    stopStream()
    setIsGenerating(false)
    setMessages((prev) => {
      const last = prev.at(-1)
      if (!last || last.role !== 'assistant') return prev
      return [
        ...prev.slice(0, -1),
        { ...last, status: 'complete' as const },
      ]
    })
  }, [stopStream])

  return (
    <div className='flex h-full min-h-0 overflow-hidden bg-[#0a0d13]'>
      <ModelListPanel
        models={models ?? []}
        isLoading={isModelsLoading}
        category={category}
        onCategoryChange={setCategory}
        search={search}
        onSearchChange={setSearch}
        selectedModelName={effectiveSelected?.name ?? null}
        onSelectModel={setSelectedModel}
      />

      <div className='flex min-w-0 flex-1 flex-col'>
        <Stage
          model={effectiveSelected}
          onOpenTaskList={() => setTaskSheetOpen(true)}
        />

        <MessageStream
          messages={messages}
          modelIcon={effectiveSelected?.icon}
          modelName={effectiveSelected?.name}
        />

        <CreatorInput
          prompt={prompt}
          onPromptChange={setPrompt}
          refs={refs}
          onRefPick={handleRefPick}
          onRefClear={handleRefClear}
          settings={settings}
          onSettingsChange={(patch) =>
            setSettings((prev) => ({ ...prev, ...patch }))
          }
          estimatedCreditsPerSecond={estimatedCredits}
          isGenerating={isGenerating || isStreaming}
          disabled={isStreaming}
          onSend={handleSend}
          onStop={handleStop}
        />
      </div>

      <TaskSheet open={taskSheetOpen} onOpenChange={setTaskSheetOpen} />
    </div>
  )
}
