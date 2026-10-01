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
  ArrowLeft,
  Clapperboard,
  Expand,
  Gift,
  Layers,
  LifeBuoy,
  ListTodo,
  Lightbulb,
  LayoutGrid,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

import type { PlazaModel } from '../types'

interface StageProps {
  model: PlazaModel | null
  onOpenTaskList: () => void
}

function StageIconButton({
  icon: Icon,
  title,
  className,
  onClick,
}: {
  icon: typeof ArrowLeft
  title?: string
  className?: string
  onClick?: () => void
}) {
  return (
    <button
      type='button'
      title={title}
      onClick={onClick}
      className={cn(
        'flex size-9 items-center justify-center rounded-full border border-white/10 bg-white/[0.05] text-gray-300 backdrop-blur transition-colors hover:border-cyan-400/40 hover:text-white',
        className
      )}
    >
      <Icon className='size-4' />
    </button>
  )
}

/**
 * Main stage: top-left task list pill + lightbulb, top-right
 * decorative icon buttons, centered model logo with radial glow and a
 * description card, plus the thin vertical toolbar on the right edge.
 */
export function Stage({ model, onOpenTaskList }: StageProps) {
  const { t } = useTranslation()
  const initial = (model?.name ?? '?')
    .trim()
    .charAt(0)
    .toUpperCase()

  return (
    <div className='relative flex min-h-0 flex-1 flex-col items-center justify-center overflow-hidden bg-[#0a0d13]'>
      {/* Subtle vignette to keep the reference feel */}
      <div className='pointer-events-none absolute inset-0 bg-[radial-gradient(ellipse_at_center,rgba(34,211,238,0.05),transparent_60%)]' />

      {/* Top-left: task list pill + lightbulb */}
      <div className='absolute top-4 left-4 flex items-center gap-2'>
        <button
          type='button'
          onClick={onOpenTaskList}
          className='flex h-8 items-center gap-1.5 rounded-full border border-white/10 bg-white/[0.05] px-3 text-xs font-medium text-gray-200 backdrop-blur transition-colors hover:border-cyan-400/40 hover:text-white'
        >
          <ListTodo className='size-3.5' />
          {t('Task List')}
        </button>
        <StageIconButton icon={Lightbulb} />
      </div>

      {/* Top-right: decorative icon buttons */}
      <div className='absolute top-4 right-4 flex items-center gap-2'>
        <StageIconButton icon={Clapperboard} />
        <div className='relative'>
          <StageIconButton icon={Gift} />
          <span className='absolute -top-1 -right-1 rounded-full bg-rose-500 px-1 text-[9px] font-bold text-white'>
            NEW
          </span>
        </div>
        <StageIconButton icon={LayoutGrid} />
      </div>

      {/* Center: model logo + glow, description card */}
      <div className='flex max-w-[640px] flex-col items-center px-6'>
        <div className='relative mb-5'>
          <div className='pointer-events-none absolute -inset-8 rounded-full bg-cyan-400/15 blur-3xl' />
          {model?.icon ? (
            <img
              src={model.icon}
              alt={model.name}
              className='relative size-[110px] rounded-3xl bg-[#0e121a] object-contain p-4 ring-1 ring-white/10 shadow-2xl'
            />
          ) : (
            <div className='relative flex size-[110px] items-center justify-center rounded-3xl bg-gradient-to-br from-cyan-500/25 via-[#10161f] to-violet-600/25 text-5xl font-bold text-cyan-200 ring-1 ring-white/10'>
              {initial}
            </div>
          )}
        </div>

        <div className='flex items-center gap-2 mb-2'>
          <h2 className='text-xl font-bold text-white tracking-tight'>
            {model?.displayName || model?.name || t('No models found')}
          </h2>
          {model?.vendorName && (
            <span className='rounded-full bg-cyan-500/10 border border-cyan-500/20 px-2.5 py-0.5 text-xs font-semibold text-cyan-300'>
              {model.vendorName}
            </span>
          )}
        </div>

        {model?.tags && model.tags.length > 0 && (
          <div className='flex flex-wrap items-center justify-center gap-1.5 mb-3'>
            {model.tags.map((tag) => (
              <span
                key={tag}
                className='rounded-md bg-white/[0.04] border border-white/[0.08] px-2 py-0.5 text-[11px] text-gray-300'
              >
                {tag}
              </span>
            ))}
          </div>
        )}

        <div className='w-full rounded-2xl border border-white/10 bg-white/[0.04] p-4 backdrop-blur-md shadow-xl'>
          <p className='text-gray-300 text-xs leading-6 text-center'>
            {model?.description?.trim() || t('No description')}
          </p>

          {/* Official Pricing Highlight */}
          {typeof model?.priceMin === 'number' && (
            <div className='mt-3 pt-3 border-t border-white/[0.08] flex items-center justify-around text-xs'>
              {model.billingMode === '按token' ? (
                <>
                  <div className='text-center'>
                    <span className='text-gray-400 block text-[10px]'>输入官方定价</span>
                    <span className='font-bold text-cyan-300'>¥{model.priceMin.toFixed(4).replace(/\.?0+$/, '')} / M tokens</span>
                  </div>
                  {typeof model.outputPriceMin === 'number' && (
                    <div className='text-center'>
                      <span className='text-gray-400 block text-[10px]'>输出官方定价</span>
                      <span className='font-bold text-amber-300'>¥{model.outputPriceMin.toFixed(4).replace(/\.?0+$/, '')} / M tokens</span>
                    </div>
                  )}
                </>
              ) : (
                <div className='text-center'>
                  <span className='text-gray-400 block text-[10px]'>官方结算单价</span>
                  <span className='font-bold text-emerald-400'>
                    ¥{model.priceMin.toFixed(4).replace(/\.?0+$/, '')} / {model.billingMode === '按秒' ? '秒' : '次'}
                  </span>
                </div>
              )}
            </div>
          )}
        </div>
      </div>

      {/* Right edge: thin vertical toolbar (decorative) */}
      <div className='absolute top-1/2 right-3 flex -translate-y-1/2 flex-col gap-2'>
        <StageIconButton icon={ArrowLeft} className='size-8' />
        <StageIconButton icon={Expand} className='size-8' />
        <StageIconButton icon={Layers} className='size-8' />
        <StageIconButton icon={LifeBuoy} className='size-8' />
      </div>
    </div>
  )
}
