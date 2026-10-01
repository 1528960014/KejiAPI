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
import type { PricingModel } from '@/features/pricing/types'

import rawLk888Models from './lk888-models.json'
import type { PlazaBadgeTone, PlazaCategory, PlazaModel } from '../types'

interface Lk888ModelRaw {
  model_id: number
  model_name: string
  display_name?: string
  intro?: string
  vendor?: string
  model_type?: 'chat' | 'image' | 'video' | 'audio'
  billing_mode?: string
  price_min?: number
  output_price_min?: number
  price_max?: number
  icon?: string
  tags?: string[]
  composite_success_rate?: number
  fastest_seconds?: number
  online_lines?: number
  protocols?: string[]
}

export const LK888_OFFICIAL_MODELS: Lk888ModelRaw[] = rawLk888Models as Lk888ModelRaw[]

export const LK888_MODELS_MAP = new Map<string, Lk888ModelRaw>(
  LK888_OFFICIAL_MODELS.map((m) => [m.model_name.toLowerCase(), m])
)

/**
 * 官方 Logo 智能映射字典
 */
export function getOfficialLogo(name: string, vendor?: string, fallbackIcon?: string): string {
  if (fallbackIcon && fallbackIcon.startsWith('http')) return fallbackIcon

  const lower = name.toLowerCase()
  const vLower = (vendor || '').toLowerCase()

  if (lower.includes('deepseek') || vLower.includes('deepseek')) return 'https://cos.lingkeai.vip/deepseek.svg'
  if (lower.startsWith('gem-') || lower.includes('gemini')) return 'https://cos.lingkeai.vip/gem.svg'
  if (lower.startsWith('claude') || lower.includes('anthropic') || vLower.includes('anthropic')) return 'https://cos.lingkeai.vip/claude.svg'
  if (lower.startsWith('gpt-') || lower.startsWith('o1') || lower.startsWith('o3') || lower.startsWith('op-') || lower.startsWith('tt-')) return 'https://cos.lingkeai.vip/op.svg'
  if (lower.startsWith('qwen') || vLower.includes('阿里') || vLower.includes('qwen')) return 'https://cos.lingkeai.vip/qwen.svg'
  if (lower.startsWith('kling') || vLower.includes('快手') || vLower.includes('kling')) return 'https://cos.lingkeai.vip/kling.svg'
  if (lower.startsWith('doubao') || lower.startsWith('seedance') || lower.startsWith('seedream') || vLower.includes('字节')) return 'https://cos.lingkeai.vip/doubao.svg'
  if (lower.startsWith('glm') || vLower.includes('智谱') || vLower.includes('zhipu')) return 'https://cos.lingkeai.vip/glm.svg'
  if (lower.startsWith('kimi') || lower.includes('moonshot') || vLower.includes('月之暗面')) return 'https://cos.lingkeai.vip/kimi.svg'
  if (lower.startsWith('minimax') || lower.startsWith('hailuo') || vLower.includes('minimax') || vLower.includes('海螺')) return 'https://cos.lingkeai.vip/minimax.svg'
  if (lower.startsWith('step') || vLower.includes('阶跃')) return 'https://cos.lingkeai.vip/step.svg'
  if (lower.startsWith('vidu') || vLower.includes('vidu')) return 'https://cos.lingkeai.vip/vidu-icon.svg'
  if (lower.startsWith('pixverse') || vLower.includes('pixverse')) return 'https://cos.lingkeai.vip/PixVerse.svg'
  if (lower.startsWith('suno') || lower.includes('music')) return 'https://cos.lingkeai.vip/suno.svg'
  if (lower.startsWith('mj_') || lower.includes('midjourney')) return 'https://cos.lingkeai.vip/Mj.svg'
  if (lower.startsWith('mimo')) return 'https://cos.lingkeai.vip/mimo_bai.svg'
  if (lower.startsWith('banana')) return 'https://cos.lingkeai.vip/banana.svg'
  if (lower.startsWith('happyhorse')) return 'https://cos.lingkeai.vip/happyhorse.svg'

  return fallbackIcon || 'https://cos.lingkeai.vip/op.svg'
}

/**
 * Heuristic model classification by name, in priority order:
 * video -> image -> audio -> chat.
 * A pattern only matches at a token boundary (start or right after a
 * non-alphanumeric character) so e.g. "wanx1.3" is not treated as the
 * "wan" video family.
 */
const CATEGORY_PATTERNS: Array<{
  category: Exclude<PlazaCategory, 'all'>
  patterns: string[]
}> = [
  {
    category: 'video',
    patterns: [
      'kling',
      'veo',
      'sora',
      'runway',
      'hailuo',
      'luma',
      'cogvideo',
      'wan',
      'video',
    ],
  },
  {
    category: 'image',
    patterns: [
      'gpt-image',
      'flux',
      'dall',
      'seedream',
      'sd',
      'stable-diffusion',
      'image',
    ],
  },
  {
    category: 'audio',
    patterns: ['tts', 'whisper', 'music', 'audio', 'speech', 'omni'],
  },
]

function matchesCategory(name: string, patterns: string[]): boolean {
  const normalized = name.toLowerCase()
  return patterns.some((pattern) =>
    new RegExp(`(^|[^a-z0-9])${pattern.replaceAll(/[.*+?^${}()|[\]\\]/g, '\\$&')}`).test(
      normalized
    )
  )
}

export function classifyPlazaModel(name: string): PlazaCategory {
  const hit = CATEGORY_PATTERNS.find((entry) =>
    matchesCategory(name, entry.patterns)
  )
  return hit ? hit.category : 'chat'
}

/**
 * Badge percentage: model ratio expressed as a percentage of the base
 * rate (ratio 1.0 = 100%). When no pricing data is available the badge
 * is left undefined and the UI renders a neutral "100%".
 */
export function computeBadgePercent(
  modelRatio?: number
): number | undefined {
  if (typeof modelRatio !== 'number' || !Number.isFinite(modelRatio)) {
    return undefined
  }
  return Math.max(1, Math.round(modelRatio * 100))
}

export function badgeToneFor(percent: number): PlazaBadgeTone {
  if (percent < 60) return 'green'
  if (percent <= 130) return 'cyan'
  return 'orange'
}

/**
 * Merge a user model name with its pricing entry (if any) into a
 * display-ready PlazaModel.
 */
export function buildPlazaModel(
  name: string,
  pricing?: PricingModel
): PlazaModel {
  const official = LK888_MODELS_MAP.get(name.toLowerCase())
  const badgePercent = computeBadgePercent(pricing?.model_ratio)
  const tone = badgePercent != null ? badgeToneFor(badgePercent) : 'cyan'

  const category = (official?.model_type as PlazaCategory) || classifyPlazaModel(name)
  const officialLogo = getOfficialLogo(name, official?.vendor, official?.icon || pricing?.icon)

  return {
    name,
    displayName: official?.display_name || name,
    category,
    description: official?.intro || pricing?.description,
    icon: officialLogo,
    vendorName: official?.vendor || pricing?.vendor_name || '大厂直供',
    modelType: (official?.model_type as PlazaModel['modelType']) || 'chat',
    billingMode: official?.billing_mode || (pricing?.quota_type === 1 ? '按次' : '按token'),
    priceMin: official?.price_min,
    outputPriceMin: official?.output_price_min,
    priceMax: official?.price_max,
    tags: official?.tags || (category === 'chat' ? ['多轮对话', '极速'] : ['创意生成']),
    successRate: official?.composite_success_rate ?? 100,
    fastestSeconds: official?.fastest_seconds,
    onlineLines: official?.online_lines ?? 5,
    protocols: official?.protocols,
    modelRatio: pricing?.model_ratio,
    completionRatio: pricing?.completion_ratio,
    modelPrice: pricing?.model_price,
    quotaType: pricing?.quota_type,
    badgePercent,
    badgeTone: tone,
  }
}
