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
import {
  ArrowRight,
  ArrowUp,
  AudioLines,
  CircleStop,
  Film,
  ImagePlus,
  Info,
  Layers,
  Sparkles,
  X,
} from 'lucide-react'
import type { ChangeEvent, KeyboardEvent } from 'react'
import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

import { ChipDropdown, type ChipOption } from './chip-dropdown'
import { formatCredits } from '../lib/estimate'
import type { PlazaRefFile, PlazaRefSelection, PlazaSettings } from '../types'

type RefSlot = 'image' | 'video' | 'audio'

const REF_SLOT_ICONS: Record<RefSlot, typeof ImagePlus> = {
  image: ImagePlus,
  video: Film,
  audio: AudioLines,
}

const REF_SLOT_META: Record<RefSlot, { accept: string; labelKey: string }> = {
  image: { accept: 'image/*', labelKey: 'Reference Image' },
  video: { accept: 'video/*', labelKey: 'Reference Video' },
  audio: { accept: 'audio/*', labelKey: 'Reference Audio' },
}

function RefSlotInput({
  slot,
  refFile,
  onPick,
  onClear,
}: {
  slot: RefSlot
  refFile: PlazaRefFile | null | undefined
  onPick: (file: PlazaRefFile) => void
  onClear: () => void
}) {
  const { t } = useTranslation()
  const meta = REF_SLOT_META[slot]
  const Icon = REF_SLOT_ICONS[slot]

  const handleFile = (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]
    if (!file) return
    onPick({
      file,
      previewUrl: slot === 'image' ? URL.createObjectURL(file) : undefined,
    })
    event.target.value = ''
  }

  return (
    <label
      className={cn(
        'group flex min-w-0 flex-1 cursor-pointer items-center gap-2 rounded-lg border border-dashed border-white/15 px-2.5 py-2 transition-colors hover:border-cyan-400/50 hover:bg-cyan-400/[0.04]',
        refFile && 'border-cyan-400/40 bg-cyan-400/[0.06]'
      )}
      title={refFile ? refFile.file.name : t(meta.labelKey)}
    >
      <input
        type='file'
        accept={meta.accept}
        className='hidden'
        onChange={handleFile}
      />
      {refFile?.previewUrl ? (
        <img
          src={refFile.previewUrl}
          alt=''
          className='size-6 shrink-0 rounded object-cover'
        />
      ) : (
        <Icon className='text-gray-500 size-4 shrink-0 group-hover:text-cyan-300' />
      )}
      <span className='min-w-0 flex-1 truncate text-xs text-gray-400 group-hover:text-gray-200'>
        {refFile ? refFile.file.name : t(meta.labelKey)}
      </span>
      {refFile ? (
        <button
          type='button'
          onClick={(event) => {
            event.preventDefault()
            if (refFile.previewUrl) URL.revokeObjectURL(refFile.previewUrl)
            onClear()
          }}
          className='text-gray-500 shrink-0 hover:text-white'
        >
          <X className='size-3.5' />
        </button>
      ) : null}
    </label>
  )
}

interface CreatorInputProps {
  prompt: string
  onPromptChange: (value: string) => void
  refs: PlazaRefSelection
  onRefPick: (slot: RefSlot, file: PlazaRefFile) => void
  onRefClear: (slot: RefSlot) => void
  settings: PlazaSettings
  onSettingsChange: (patch: Partial<PlazaSettings>) => void
  estimatedCreditsPerSecond: number
  isGenerating: boolean
  disabled: boolean
  onSend: () => void
  onStop: () => void
}

export function CreatorInput({
  prompt,
  onPromptChange,
  refs,
  onRefPick,
  onRefClear,
  settings,
  onSettingsChange,
  estimatedCreditsPerSecond,
  isGenerating,
  disabled,
  onSend,
  onStop,
}: CreatorInputProps) {
  const { t } = useTranslation()

  const qualityOptions: ChipOption[] = [
    { value: 'best', label: t('Best Overall') },
    { value: 'high', label: t('High Quality') },
    { value: 'fast', label: t('Fast') },
  ]
  const countOptions: ChipOption[] = [1, 2, 4].map((value) => ({
    value: String(value),
    label: t('{{count}} items', { count: value }),
  }))
  const modeOptions: ChipOption[] = [
    { value: 'text', label: t('Text to Video') },
    { value: 'firstLast', label: t('First/Last Frame') },
    { value: 'reference', label: t('Reference to Video') },
  ]
  const durationOptions: ChipOption[] = [5, 10, 15, 30].map((value) => ({
    value: String(value),
    label: t('{{count}} sec', { count: value }),
  }))
  const ratioOptions: ChipOption[] = [
    { value: 'auto', label: t('Adaptive') },
    { value: '16:9', label: '16:9' },
    { value: '9:16', label: '9:16' },
    { value: '1:1', label: '1:1' },
  ]
  const resolutionOptions: ChipOption[] = [
    { value: '720p', label: '720p' },
    { value: '1080p', label: '1080p' },
  ]
  const watermarkOptions: ChipOption[] = [
    { value: 'off', label: t('Off') },
    { value: 'on', label: t('On') },
  ]

  const canSend = !disabled && prompt.trim().length > 0

  const handleKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key === 'Enter' && !event.shiftKey) {
      event.preventDefault()
      if (canSend) onSend()
    }
  }

  return (
    <div className='relative mx-auto w-full max-w-[800px] px-4 pb-4'>
      {/* Estimated cost chip floating on the card edge */}
      <div
        className='absolute -top-3 right-6 z-10'
        title={t('Cost estimate is approximate')}
      >
        <span className='flex items-center gap-1 rounded-full border border-cyan-400/30 bg-[#0d1118] px-2.5 py-1 text-[11px] text-cyan-200 shadow-[0_0_12px_rgba(34,211,238,0.2)]'>
          {t('Estimated {{credits}}/sec', {
            credits: formatCredits(estimatedCreditsPerSecond),
          })}
          <Info className='size-3 text-cyan-400/70' />
        </span>
      </div>

      <div className='rounded-2xl border border-white/10 bg-[#10141c] p-3 shadow-[0_10px_40px_rgba(0,0,0,0.45)]'>
        {/* Reference upload slots */}
        <div className='mb-3 flex items-center gap-1.5'>
          <RefSlotInput
            slot='image'
            refFile={refs.image}
            onPick={(file) => onRefPick('image', file)}
            onClear={() => onRefClear('image')}
          />
          <ArrowRight className='text-gray-600 size-3.5 shrink-0' />
          <RefSlotInput
            slot='video'
            refFile={refs.video}
            onPick={(file) => onRefPick('video', file)}
            onClear={() => onRefClear('video')}
          />
          <ArrowRight className='text-gray-600 size-3.5 shrink-0' />
          <RefSlotInput
            slot='audio'
            refFile={refs.audio}
            onPick={(file) => onRefPick('audio', file)}
            onClear={() => onRefClear('audio')}
          />
        </div>

        {/* Prompt textarea */}
        <div className='relative'>
          <textarea
            value={prompt}
            onChange={(event) => onPromptChange(event.target.value)}
            onKeyDown={handleKeyDown}
            disabled={disabled}
            rows={3}
            placeholder={t(
              'Describe the scene, motion and atmosphere you want, and specify the video duration in seconds (billed per second). First/last frame mode takes 1~2 images; reference mode accepts reference image / reference video / reference audio (audio only is fine); upload nothing for text-to-video.'
            )}
            className='max-h-40 min-h-[72px] w-full resize-none bg-transparent text-sm text-white placeholder:text-gray-500 focus:outline-none disabled:opacity-60'
          />
          <Layers className='pointer-events-none absolute top-2 right-0 size-3.5 text-gray-600' />
        </div>

        {/* Option chips + actions */}
        <div className='mt-2.5 flex flex-wrap items-center gap-1.5'>
          <ChipDropdown
            label={t('Quality')}
            value={settings.quality}
            options={qualityOptions}
            onChange={(value) =>
              onSettingsChange({ quality: value as PlazaSettings['quality'] })
            }
          />
          <ChipDropdown
            label={t('Count')}
            value={String(settings.count)}
            options={countOptions}
            onChange={(value) =>
              onSettingsChange({
                count: Number(value) as PlazaSettings['count'],
              })
            }
          />
          <ChipDropdown
            label={t('Mode')}
            value={settings.mode}
            options={modeOptions}
            onChange={(value) =>
              onSettingsChange({ mode: value as PlazaSettings['mode'] })
            }
          />
          <ChipDropdown
            label={t('Video Duration')}
            value={String(settings.duration)}
            options={durationOptions}
            onChange={(value) =>
              onSettingsChange({
                duration: Number(value) as PlazaSettings['duration'],
              })
            }
          />
          <ChipDropdown
            label={t('Aspect Ratio')}
            value={settings.ratio}
            options={ratioOptions}
            onChange={(value) =>
              onSettingsChange({ ratio: value as PlazaSettings['ratio'] })
            }
          />
          <ChipDropdown
            value={settings.resolution}
            options={resolutionOptions}
            onChange={(value) =>
              onSettingsChange({
                resolution: value as PlazaSettings['resolution'],
              })
            }
          />
          <ChipDropdown
            label={t('Watermark')}
            value={settings.watermark}
            options={watermarkOptions}
            onChange={(value) =>
              onSettingsChange({
                watermark: value as PlazaSettings['watermark'],
              })
            }
          />

          <div className='ml-auto flex items-center gap-2'>
            <button
              type='button'
              disabled
              title='AI'
              className='flex h-7 items-center gap-1 rounded-md bg-gradient-to-r from-orange-400 to-amber-500 px-2 text-[11px] font-bold text-orange-950 opacity-80'
            >
              <Sparkles className='size-3' />
              AI
            </button>
            {isGenerating ? (
              <button
                type='button'
                onClick={onStop}
                title={t('Stop')}
                className='flex size-9 items-center justify-center rounded-full border border-cyan-400/40 bg-cyan-400/10 text-cyan-200 transition-colors hover:bg-cyan-400/20'
              >
                <CircleStop className='size-4' />
              </button>
            ) : (
              <button
                type='button'
                onClick={onSend}
                disabled={!canSend}
                title={t('Send')}
                className={cn(
                  'flex size-9 items-center justify-center rounded-full transition-all',
                  canSend
                    ? 'bg-cyan-400 text-[#06282e] shadow-[0_0_18px_rgba(34,211,238,0.45)] hover:bg-cyan-300'
                    : 'cursor-not-allowed bg-white/10 text-gray-500'
                )}
              >
                <ArrowUp className='size-4' />
              </button>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
