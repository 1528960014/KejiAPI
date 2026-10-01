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
  Activity,
  Check,
  ChevronDown,
  Coins,
  ImagePlus,
  Search,
  Sparkles,
  Video,
  Wallet,
  Zap,
} from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Skeleton } from '@/components/ui/skeleton'
import { getUserAvatarFallback, getUserAvatarStyle } from '@/lib/avatar'
import { formatQuotaWithCurrency } from '@/lib/currency'
import { cn } from '@/lib/utils'
import { useUserDisplay } from '@/hooks/use-user-display'
import { useAuthStore } from '@/stores/auth-store'

import { RechargeDialog } from './recharge-dialog'
import type { PlazaCategory, PlazaModel } from '../types'

const CATEGORIES: Array<{ id: PlazaCategory; labelKey: string }> = [
  { id: 'all', labelKey: 'All' },
  { id: 'chat', labelKey: 'Chat' },
  { id: 'image', labelKey: 'Image' },
  { id: 'video', labelKey: 'Video' },
  { id: 'audio', labelKey: 'Audio' },
]

const VENDORS = [
  'All Vendors',
  '大厂直供',
  '阿里巴巴',
  'DeepSeek',
  '快手',
  '字节跳动',
  '智谱AI',
  '月之暗面',
  'MiniMax',
  '阶跃星辰',
  '小米',
  'vidu',
  'MidJourney',
  'Suno',
]

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
  const title = model.displayName || model.name

  // 格式化官方定价展示
  const renderPricing = () => {
    if (model.billingMode === '按次' && typeof model.priceMin === 'number') {
      return (
        <span className='inline-flex items-center gap-1 text-[11px] font-medium text-emerald-400'>
          <Coins className='size-3' />
          ¥{model.priceMin.toFixed(4).replace(/\.?0+$/, '')} / 次
        </span>
      )
    }
    if (model.billingMode === '按秒' && typeof model.priceMin === 'number') {
      return (
        <span className='inline-flex items-center gap-1 text-[11px] font-medium text-purple-400'>
          <Coins className='size-3' />
          ¥{model.priceMin.toFixed(4).replace(/\.?0+$/, '')} / 秒
        </span>
      )
    }
    if (typeof model.priceMin === 'number') {
      return (
        <div className='flex items-center gap-2 text-[10px] text-gray-400'>
          <span className='text-cyan-300 font-medium'>
            入 ¥{model.priceMin.toFixed(4).replace(/\.?0+$/, '')}/M
          </span>
          {typeof model.outputPriceMin === 'number' && (
            <span className='text-amber-300 font-medium'>
              出 ¥{model.outputPriceMin.toFixed(4).replace(/\.?0+$/, '')}/M
            </span>
          )}
        </div>
      )
    }
    return (
      <span className='text-[10px] text-gray-500 font-mono'>
        {model.modelRatio != null ? `倍率 ${(model.modelRatio).toFixed(2)}x` : '官方标准费率'}
      </span>
    )
  }

  return (
    <button
      type='button'
      onClick={onSelect}
      className={cn(
        'group relative w-full rounded-2xl border p-3.5 text-left transition-all backdrop-blur-sm',
        selected
          ? 'border-cyan-400/80 bg-cyan-500/[0.08] shadow-[0_0_20px_rgba(34,211,238,0.22)] ring-1 ring-cyan-400/50'
          : 'border-white/[0.07] bg-white/[0.02] hover:border-white/20 hover:bg-white/[0.05]'
      )}
    >
      {/* Top right badges: success rate + status */}
      <div className='absolute top-3 right-3 flex items-center gap-1.5'>
        {model.successRate != null && (
          <span className='inline-flex items-center gap-0.5 rounded-full bg-emerald-500/10 border border-emerald-500/20 px-1.5 py-0.5 text-[9px] font-bold text-emerald-400'>
            <Activity className='size-2.5' />
            {model.successRate}%
          </span>
        )}
        <span className='size-2 rounded-full bg-emerald-400 shadow-[0_0_8px_rgba(52,211,153,0.8)]' />
      </div>

      <div className='flex items-start gap-3.5 pr-14'>
        {/* Official SVG Logo */}
        {model.icon ? (
          <div className='relative size-12 shrink-0 rounded-xl bg-[#090b10] p-1.5 ring-1 ring-white/10 flex items-center justify-center shadow-inner'>
            <img
              src={model.icon}
              alt={model.name}
              className='size-full object-contain'
              loading='lazy'
            />
          </div>
        ) : (
          <span
            className={cn(
              'flex size-12 shrink-0 items-center justify-center rounded-xl bg-gradient-to-br ring-1 ring-white/10',
              CATEGORY_ICON_CLASSES[model.category]
            )}
          >
            <CategoryIcon category={model.category} />
          </span>
        )}

        {/* Info */}
        <div className='min-w-0 flex-1'>
          <div className='flex items-center gap-1.5'>
            <span className='truncate text-sm font-bold text-white group-hover:text-cyan-300 transition-colors'>
              {title}
            </span>
            {model.vendorName && (
              <span className='shrink-0 rounded bg-white/[0.08] px-1.5 py-0.5 text-[10px] font-medium text-gray-300'>
                {model.vendorName}
              </span>
            )}
          </div>

          <span className='block truncate text-[11px] font-mono text-gray-500 mt-0.5'>
            {model.name}
          </span>

          {/* Tags */}
          {model.tags && model.tags.length > 0 && (
            <div className='flex flex-wrap gap-1 mt-1.5'>
              {model.tags.slice(0, 3).map((tag) => (
                <span
                  key={tag}
                  className='rounded bg-cyan-950/40 border border-cyan-800/30 px-1.5 py-0.5 text-[9px] text-cyan-300/90'
                >
                  {tag}
                </span>
              ))}
            </div>
          )}

          {/* Pricing bar */}
          <div className='mt-2 pt-2 border-t border-white/[0.05] flex items-center justify-between'>
            {renderPricing()}
            {model.onlineLines != null && model.onlineLines > 0 && (
              <span className='text-[10px] text-gray-500 font-medium'>
                {model.onlineLines} 线路
              </span>
            )}
          </div>
        </div>
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
  const user = useAuthStore((state) => state.auth.user)
  const { displayName } = useUserDisplay(user)
  const avatarName = user?.username || displayName
  const avatarFallback = getUserAvatarFallback(avatarName)
  const avatarStyle = useMemo(
    () => getUserAvatarStyle(avatarName),
    [avatarName]
  )

  const [selectedVendor, setSelectedVendor] = useState<string>('All Vendors')
  const [rechargeOpen, setRechargeOpen] = useState<boolean>(false)

  const filtered = useMemo(() => {
    const query = search.trim().toLowerCase()
    return models.filter((model) => {
      if (category !== 'all' && model.category !== category) return false
      if (selectedVendor !== 'All Vendors' && model.vendorName !== selectedVendor) {
        return false
      }
      if (!query) return true
      return (
        model.name.toLowerCase().includes(query) ||
        (model.displayName ?? '').toLowerCase().includes(query) ||
        (model.description ?? '').toLowerCase().includes(query) ||
        (model.vendorName ?? '').toLowerCase().includes(query)
      )
    })
  }, [models, category, search, selectedVendor])

  const renderListBody = () => {
    if (isLoading) {
      return (
        <div className='space-y-2.5 pt-1'>
          {MODEL_SKELETONS.map((skeleton) => (
            <Skeleton
              key={skeleton.id}
              className='h-[92px] w-full rounded-2xl bg-white/[0.04]'
            />
          ))}
        </div>
      )
    }
    if (filtered.length === 0) {
      return (
        <p className='text-gray-500 px-2 pt-12 text-center text-xs'>
          {t('No models found')}
        </p>
      )
    }
    return (
      <div className='space-y-2.5'>
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
    <>
      <aside className='flex h-full w-[380px] shrink-0 flex-col border-r border-white/[0.08] bg-[#0c0e14]'>
        {/* Category tabs */}
        <div className='flex gap-1.5 px-3.5 pt-3.5 pb-2.5'>
          {CATEGORIES.map((item) => (
            <button
              key={item.id}
              type='button'
              onClick={() => onCategoryChange(item.id)}
              className={cn(
                'h-7 flex-1 rounded-lg text-xs font-semibold transition-all',
                category === item.id
                  ? 'bg-cyan-500 text-white shadow-[0_0_15px_rgba(34,211,238,0.4)]'
                  : 'text-gray-400 hover:bg-white/[0.06] hover:text-gray-200'
              )}
            >
              {t(item.labelKey)}
            </button>
          ))}
        </div>

        {/* Vendor selector + search */}
        <div className='flex items-center gap-2 px-3.5 pb-3'>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <button
                type='button'
                className='flex h-8 shrink-0 items-center gap-1.5 rounded-xl border border-white/10 bg-white/[0.04] px-2.5 text-xs font-medium text-gray-200 transition-colors hover:border-cyan-400/40 hover:text-white'
              >
                {selectedVendor === 'All Vendors' ? t('All Vendors') : selectedVendor}
                <ChevronDown className='text-gray-400 size-3' />
              </button>
            </DropdownMenuTrigger>
            <DropdownMenuContent
              align='start'
              className='max-h-60 overflow-y-auto bg-[#131620] border-white/10 text-white shadow-2xl rounded-xl p-1'
            >
              {VENDORS.map((v) => (
                <DropdownMenuItem
                  key={v}
                  onClick={() => setSelectedVendor(v)}
                  className='flex items-center justify-between text-xs cursor-pointer py-1.5 px-2.5 rounded-lg hover:bg-white/10'
                >
                  <span>{v === 'All Vendors' ? t('All Vendors') : v}</span>
                  {selectedVendor === v && <Check className='size-3 text-cyan-400' />}
                </DropdownMenuItem>
              ))}
            </DropdownMenuContent>
          </DropdownMenu>

          <div className='relative min-w-0 flex-1'>
            <Search className='pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-gray-500' />
            <input
              type='text'
              value={search}
              onChange={(event) => onSearchChange(event.target.value)}
              placeholder={t('Search models, vendors, tags...')}
              className='h-8 w-full rounded-xl border border-white/10 bg-white/[0.04] pr-2.5 pl-8 text-xs text-white placeholder:text-gray-500 focus:border-cyan-400/50 focus:bg-white/[0.06] focus:outline-none transition-all'
            />
          </div>
        </div>

        {/* Model cards list */}
        <div className='min-h-0 flex-1 overflow-y-auto px-3.5 pb-3.5 [scrollbar-width:thin] [scrollbar-color:rgba(255,255,255,0.15)_transparent]'>
          {renderListBody()}
        </div>

        {/* User profile + Quick Recharge button */}
        <div className='flex items-center gap-3 border-t border-white/[0.08] bg-[#090b10] px-3.5 py-3'>
          <Avatar className='size-9 shrink-0 ring-1 ring-white/10'>
            <AvatarFallback
              className='text-xs font-semibold text-white'
              style={avatarStyle}
            >
              {avatarFallback}
            </AvatarFallback>
          </Avatar>
          <div className='min-w-0 flex-1'>
            <p className='truncate text-sm font-semibold text-white'>
              {displayName}
            </p>
            <p className='flex items-center gap-1 text-xs text-gray-400 mt-0.5'>
              <Zap className='text-amber-300 size-3' />
              {formatQuotaWithCurrency(user?.quota ?? 0)}
            </p>
          </div>
          <button
            type='button'
            onClick={() => setRechargeOpen(true)}
            className='flex h-8 shrink-0 items-center gap-1.5 rounded-xl bg-gradient-to-r from-amber-400 via-amber-300 to-yellow-400 px-3 text-xs font-bold text-amber-950 shadow-[0_0_15px_rgba(251,191,36,0.35)] transition-all hover:opacity-90 hover:scale-[1.02] active:scale-[0.98]'
          >
            <Wallet className='size-3.5' />
            {t('Recharge')}
          </button>
        </div>
      </aside>

      {/* Recharge Modal */}
      <RechargeDialog open={rechargeOpen} onOpenChange={setRechargeOpen} />
    </>
  )
}
