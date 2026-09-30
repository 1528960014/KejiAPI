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
import { useNavigate } from '@tanstack/react-router'
import {
  ChevronDown,
  ImagePlus,
  Search,
  Sparkles,
  Video,
  Wallet,
  Zap,
} from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import { Skeleton } from '@/components/ui/skeleton'
import { formatQuotaWithCurrency } from '@/lib/currency'
import { getUserAvatarFallback, getUserAvatarStyle } from '@/lib/avatar'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'
import { useUserDisplay } from '@/hooks/use-user-display'

import type { PlazaCategory, PlazaModel } from '../types'

const CATEGORIES: Array<{ id: PlazaCategory; labelKey: string }> = [
  { id: 'all', labelKey: 'All' },
  { id: 'chat', labelKey: 'Chat' },
  { id: 'image', labelKey: 'Image' },
  { id: 'video', labelKey: 'Video' },
  { id: 'audio', labelKey: 'Audio' },
]

const BADGE_TONE_CLASSES: Record<PlazaModel['badgeTone'], string> = {
  green: 'bg-emerald-500/15 text-emerald-300 ring-emerald-400/30',
  cyan: 'bg-cyan-500/15 text-cyan-300 ring-cyan-400/30',
  orange: 'bg-orange-500/15 text-orange-300 ring-orange-400/30',
}

const CATEGORY_ICON_CLASSES: Record<PlazaCategory, string> = {
  all: 'from-slate-500/40 to-slate-700/40',
  chat: 'from-cyan-500/30 to-blue-700/30',
  image: 'from-violet-500/30 to-fuchsia-700/30',
  video: 'from-emerald-500/30 to-teal-700/30',
  audio: 'from-amber-500/30 to-orange-700/30',
}

const CATEGORY_ICON_TEXT_CLASSES: Record<PlazaCategory, string> = {
  all: 'size-5 text-cyan-300',
  chat: 'size-5 text-cyan-300',
  image: 'size-5 text-violet-300',
  video: 'size-5 text-emerald-300',
  audio: 'size-5 text-amber-300',
}

const MODEL_SKELETONS = Array.from(
  { length: 6 },
  (_, index) => ({ id: `model-card-skeleton-${index + 1}` })
)

function CategoryIcon({ category }: { category: PlazaCategory }) {
  const iconClass = CATEGORY_ICON_TEXT_CLASSES[category]
  if (category === 'image') return <ImagePlus className={iconClass} />
  if (category === 'video') return <Video className={iconClass} />
  if (category === 'audio') return <Zap className={iconClass} />
  return <Sparkles className={iconClass} />
}

interface ModelCardProps {
  model: PlazaModel
  selected: boolean
  onSelect: () => void
}

function ModelCard({ model, selected, onSelect }: ModelCardProps) {
  const { t } = useTranslation()
  const badge = model.badgePercent ?? 100

  return (
    <button
      type='button'
      onClick={onSelect}
      className={cn(
        'group relative w-full rounded-xl border p-3 text-left transition-all',
        selected
          ? 'border-cyan-400/60 bg-cyan-400/[0.06] shadow-[0_0_0_1px_rgba(34,211,238,0.35),0_0_20px_rgba(34,211,238,0.18)]'
          : 'border-white/[0.07] bg-white/[0.02] hover:border-white/15 hover:bg-white/[0.05]'
      )}
    >
      <span
        className={cn(
          'absolute top-2.5 right-2.5 rounded-md px-1.5 py-0.5 text-[11px] font-medium ring-1',
          BADGE_TONE_CLASSES[model.badgeTone]
        )}
      >
        {badge}%
      </span>
      <div className='flex items-start gap-3 pr-12'>
        {model.icon ? (
          <img
            src={model.icon}
            alt=''
            className='size-12 shrink-0 rounded-lg bg-[#0d1118] object-contain p-1 ring-1 ring-white/10'
          />
        ) : (
          <span
            className={cn(
              'flex size-12 shrink-0 items-center justify-center rounded-lg bg-gradient-to-br ring-1 ring-white/10',
              CATEGORY_ICON_CLASSES[model.category]
            )}
          >
            <CategoryIcon category={model.category} />
          </span>
        )}
        <span className='min-w-0 flex-1'>
          <span className='block truncate text-sm font-semibold text-white'>
            {model.name}
          </span>
          <span className='text-gray-400 mt-1 line-clamp-2 block text-xs leading-5'>
            {model.description?.trim() || t('No description')}
          </span>
        </span>
      </div>
    </button>
  )
}

interface ModelListPanelProps {
  models: PlazaModel[]
  isLoading: boolean
  category: PlazaCategory
  onCategoryChange: (category: PlazaCategory) => void
  search: string
  onSearchChange: (value: string) => void
  selectedModelName: string | null
  onSelectModel: (model: PlazaModel) => void
}

export function ModelListPanel({
  models,
  isLoading,
  category,
  onCategoryChange,
  search,
  onSearchChange,
  selectedModelName,
  onSelectModel,
}: ModelListPanelProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const user = useAuthStore((state) => state.auth.user)
  const { displayName } = useUserDisplay(user)
  const avatarName = user?.username || displayName
  const avatarFallback = getUserAvatarFallback(avatarName)
  const avatarStyle = useMemo(
    () => getUserAvatarStyle(avatarName),
    [avatarName]
  )

  const filtered = useMemo(() => {
    const query = search.trim().toLowerCase()
    return models.filter((model) => {
      if (category !== 'all' && model.category !== category) return false
      if (!query) return true
      return (
        model.name.toLowerCase().includes(query) ||
        (model.description ?? '').toLowerCase().includes(query)
      )
    })
  }, [models, category, search])

  const renderListBody = () => {
    if (isLoading) {
      return (
        <div className='space-y-2 pt-1'>
          {MODEL_SKELETONS.map((skeleton) => (
            <Skeleton
              key={skeleton.id}
              className='h-[74px] w-full rounded-xl bg-white/[0.05]'
            />
          ))}
        </div>
      )
    }
    if (filtered.length === 0) {
      return (
        <p className='text-gray-500 px-2 pt-8 text-center text-xs'>
          {t('No models found')}
        </p>
      )
    }
    return (
      <div className='space-y-2'>
        {filtered.map((model) => (
          <ModelCard
            key={model.name}
            model={model}
            selected={model.name === selectedModelName}
            onSelect={() => onSelectModel(model)}
          />
        ))}
      </div>
    )
  }

  return (
    <aside className='flex h-full w-[340px] shrink-0 flex-col border-r border-white/[0.07] bg-[#0d1118]'>
      {/* Category tabs */}
      <div className='flex gap-1.5 px-3 pt-3 pb-2'>
        {CATEGORIES.map((item) => (
          <button
            key={item.id}
            type='button'
            onClick={() => onCategoryChange(item.id)}
            className={cn(
              'h-7 flex-1 rounded-md text-xs font-medium transition-colors',
              category === item.id
                ? 'bg-cyan-500 text-white shadow-[0_0_14px_rgba(34,211,238,0.35)]'
                : 'text-gray-400 hover:bg-white/[0.06] hover:text-gray-200'
            )}
          >
            {t(item.labelKey)}
          </button>
        ))}
      </div>

      {/* Vendor selector + search */}
      <div className='flex items-center gap-2 px-3 pb-3'>
        <button
          type='button'
          className='flex h-8 shrink-0 items-center gap-1.5 rounded-lg border border-white/10 bg-white/[0.04] px-2.5 text-xs text-gray-300 transition-colors hover:border-cyan-400/40 hover:text-white'
        >
          {t('All Vendors')}
          <ChevronDown className='text-gray-500 size-3' />
        </button>
        <div className='relative min-w-0 flex-1'>
          <Search className='pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-gray-500' />
          <input
            type='text'
            value={search}
            onChange={(event) => onSearchChange(event.target.value)}
            placeholder={t('Search models or features...')}
            className='h-8 w-full rounded-lg border border-white/10 bg-white/[0.04] pr-2 pl-8 text-xs text-white placeholder:text-gray-500 focus:border-cyan-400/50 focus:outline-none'
          />
        </div>
      </div>

      {/* Model cards */}
      <div className='min-h-0 flex-1 overflow-y-auto px-3 pb-3 [scrollbar-width:thin] [scrollbar-color:rgba(255,255,255,0.15)_transparent]'>
        {renderListBody()}
      </div>

      {/* User card + recharge */}
      <div className='flex items-center gap-2.5 border-t border-white/[0.07] px-3 py-3'>
        <Avatar className='size-9 shrink-0'>
          <AvatarFallback
            className='text-xs font-semibold text-white'
            style={avatarStyle}
          >
            {avatarFallback}
          </AvatarFallback>
        </Avatar>
        <div className='min-w-0 flex-1'>
          <p className='truncate text-sm font-medium text-white'>
            {displayName}
          </p>
          <p className='flex items-center gap-1 text-xs text-gray-400'>
            <Zap className='text-amber-300 size-3' />
            {formatQuotaWithCurrency(user?.quota ?? 0)}
          </p>
        </div>
        <button
          type='button'
          onClick={() => navigate({ to: '/wallet' })}
          className='flex h-7 shrink-0 items-center gap-1 rounded-lg bg-gradient-to-r from-amber-300 to-yellow-400 px-2.5 text-xs font-semibold text-amber-950 shadow-[0_0_14px_rgba(251,191,36,0.35)] transition-opacity hover:opacity-90'
        >
          <Wallet className='size-3.5' />
          {t('Recharge')}
        </button>
      </div>
    </aside>
  )
}
