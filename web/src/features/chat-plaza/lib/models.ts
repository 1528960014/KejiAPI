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
import rawSubKejikeModels from './sub_kejike_processed_models.json'
import type { PlazaBadgeTone, PlazaCategory, PlazaModel } from '../types'

interface ModelMetaRaw {
  model_id?: number
  model_name: string
  display_name?: string
  intro?: string
  vendor?: string
  platform?: string
  group_name?: string
  group_multiplier?: number
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

export const SUB_KEJIKE_MODELS: ModelMetaRaw[] = rawSubKejikeModels as ModelMetaRaw[]
export const LK888_OFFICIAL_MODELS: ModelMetaRaw[] = rawLk888Models as ModelMetaRaw[]

// Combined list: sub.kejike.top groups first, then general official catalog
export const ALL_OFFICIAL_MODELS: ModelMetaRaw[] = [
  ...SUB_KEJIKE_MODELS,
  ...LK888_OFFICIAL_MODELS,
]

export const ALL_MODELS_MAP = new Map<string, ModelMetaRaw>()
for (const m of ALL_OFFICIAL_MODELS) {
  const k = m.model_name.toLowerCase()
  if (!ALL_MODELS_MAP.has(k)) {
    ALL_MODELS_MAP.set(k, m)
  }
}

/**
 * 规范化格式化模型展示名称 (去除杂乱日期后缀或内部代码，呈现官方名称)
 */
export function formatModelDisplayName(rawName: string, existingDisplay?: string): string {
  if (existingDisplay && existingDisplay !== rawName && !existingDisplay.includes('????')) {
    return existingDisplay
  }
  const lower = rawName.toLowerCase()
  if (lower.includes('claude-3-7-sonnet') || lower.includes('claude-3.7-sonnet')) return 'Claude 3.7 Sonnet'
  if (lower.includes('claude-3-5-sonnet') || lower.includes('claude-3.5-sonnet')) return 'Claude 3.5 Sonnet'
  if (lower.includes('claude-3-5-haiku') || lower.includes('claude-3.5-haiku')) return 'Claude 3.5 Haiku'
  if (lower.includes('claude-3-opus') || lower.includes('claude-3.0-opus')) return 'Claude 3 Opus'
  if (lower.includes('claude-3-haiku') || lower.includes('claude-3.0-haiku')) return 'Claude 3 Haiku'
  if (lower.includes('claude-fable-5-1')) return 'Claude Fable 5.1'
  if (lower.includes('claude-fable-5')) return 'Claude Fable 5'
  if (lower === 'gpt-4o' || lower.startsWith('gpt-4o-202')) return 'GPT-4o'
  if (lower.includes('gpt-4o-mini')) return 'GPT-4o Mini'
  if (lower.includes('gpt-4.5') || lower.includes('gpt-4-5')) return 'GPT-4.5 Preview'
  if (lower.includes('chatgpt-4o-latest')) return 'ChatGPT-4o Latest'
  if (lower.includes('o1-mini')) return 'OpenAI o1 Mini'
  if (lower.includes('o1-preview') || lower === 'o1') return 'OpenAI o1'
  if (lower.includes('o3-mini')) return 'OpenAI o3 Mini'
  if (lower.includes('o3')) return 'OpenAI o3'
  if (lower.includes('deepseek-r1') || lower.includes('deepseek-reasoner')) return 'DeepSeek R1'
  if (lower.includes('deepseek-v3') || lower.includes('deepseek-chat')) return 'DeepSeek V3'
  if (lower.includes('deepseek-v4')) return 'DeepSeek V4 Flash'
  if (lower.includes('gemini-2.5-flash')) return 'Gemini 2.5 Flash'
  if (lower.includes('gemini-2.5-pro')) return 'Gemini 2.5 Pro'
  if (lower.includes('gemini-2.0-flash')) return 'Gemini 2.0 Flash'
  if (lower.includes('gemini-1.5-pro')) return 'Gemini 1.5 Pro'
  if (lower.includes('gemini-1.5-flash')) return 'Gemini 1.5 Flash'
  if (lower.includes('qwen-max')) return '通义千问 Max'
  if (lower.includes('qwen-plus')) return '通义千问 Plus'
  if (lower.includes('qwen-turbo')) return '通义千问 Turbo'
  if (lower.includes('kimi-k2.6') || lower.includes('kimi-k2')) return 'Kimi K2.6'
  if (lower.includes('kimi-k3')) return 'Kimi K3'
  if (lower.includes('glm-5')) return 'GLM-5'
  if (lower.includes('glm-4')) return 'GLM-4'
  if (lower.includes('grok-4.5')) return 'Grok 4.5'
  if (lower.includes('grok-4.3')) return 'Grok 4.3'
  return rawName
}

/**
 * 官方 Logo 智能映射字典 (100% 精确官方 SVG / 高清图标)
 */
export function getOfficialLogo(name: string, vendor?: string, fallbackIcon?: string): string {
  const lower = name.toLowerCase()
  const vLower = (vendor || '').toLowerCase()

  // 1. Claude 系列 (Anthropic 官方图标)
  if (lower.includes('claude') || lower.includes('fable') || vLower.includes('anthropic')) {
    return 'https://cos.lingkeai.vip/claude.svg'
  }

  // 2. OpenAI / GPT / Codex 系列 (OpenAI 官方图标)
  if (
    lower.startsWith('gpt') ||
    lower.startsWith('o1') ||
    lower.startsWith('o3') ||
    lower.startsWith('op-') ||
    lower.startsWith('tt-') ||
    lower.includes('codex') ||
    lower.includes('chatgpt') ||
    vLower.includes('openai')
  ) {
    return 'https://cos.lingkeai.vip/op.svg'
  }

  // 3. Google Gemini 系列 (Google 官方极光四角星 SVG)
  if (lower.startsWith('gem-') || lower.includes('gemini') || vLower.includes('google')) {
    return 'https://cos.lingkeai.vip/gem.svg'
  }

  // 4. DeepSeek 系列 (DeepSeek 官方鲸鱼蓝标)
  if (lower.includes('deepseek') || vLower.includes('deepseek')) {
    return 'https://cos.lingkeai.vip/deepseek.svg'
  }

  // 5. Kimi 系列 (月之暗面官方标志)
  if (lower.includes('kimi') || lower.includes('moonshot') || vLower.includes('月之暗面')) {
    return 'https://cos.lingkeai.vip/kimi.svg'
  }

  // 6. 智谱 GLM 系列 (智谱官方标志)
  if (lower.includes('glm') || lower.includes('zhipu') || vLower.includes('智谱')) {
    return 'https://cos.lingkeai.vip/glm.svg'
  }

  // 7. 通义千问 Qwen 系列 (通义官方彩色图标)
  if (lower.includes('qwen') || vLower.includes('阿里') || vLower.includes('通义')) {
    return 'https://cos.lingkeai.vip/qwen.svg'
  }

  // 8. 可灵 Kling 系列 (快手官方图标)
  if (lower.includes('kling') || vLower.includes('快手')) {
    return 'https://cos.lingkeai.vip/kling.svg'
  }

  // 9. 豆包 / 即梦 / Seedance 系列 (字节跳动官方图标)
  if (
    lower.includes('doubao') ||
    lower.includes('seedance') ||
    lower.includes('seedream') ||
    lower.includes('jimeng') ||
    vLower.includes('字节')
  ) {
    return 'https://cos.lingkeai.vip/doubao.svg'
  }

  // 10. MiniMax / 海螺 AI (MiniMax 官方图标)
  if (lower.includes('minimax') || lower.includes('hailuo') || vLower.includes('minimax') || vLower.includes('海螺')) {
    return 'https://cos.lingkeai.vip/minimax.svg'
  }

  // 11. 阶跃星辰 StepFun (Step 官方图标)
  if (lower.includes('step') || vLower.includes('阶跃')) {
    return 'https://cos.lingkeai.vip/step.svg'
  }

  // 12. Vidu (Vidu 官方图标)
  if (lower.includes('vidu') || vLower.includes('vidu')) {
    return 'https://cos.lingkeai.vip/vidu-icon.svg'
  }

  // 13. PixVerse (PixVerse 官方图标)
  if (lower.includes('pixverse') || vLower.includes('pixverse')) {
    return 'https://cos.lingkeai.vip/PixVerse.svg'
  }

  // 14. MidJourney
  if (lower.includes('mj_') || lower.includes('midjourney')) {
    return 'https://cos.lingkeai.vip/Mj.svg'
  }

  // 15. Suno (Suno 音乐官方图标)
  if (lower.includes('suno') || lower.includes('music')) {
    return 'https://cos.lingkeai.vip/suno.svg'
  }

  // 16. Banana / Nano-Banana
  if (lower.includes('banana')) {
    return 'https://cos.lingkeai.vip/banana.svg'
  }

  // 17. Xiaomi Mimo
  if (lower.includes('mimo')) {
    return 'https://cos.lingkeai.vip/mimo_bai.svg'
  }

  // 18. Grok / xAI
  if (lower.includes('grok') || vLower.includes('xai')) {
    return 'https://cos.lingkeai.vip/op.svg'
  }

  if (fallbackIcon && fallbackIcon.startsWith('http')) return fallbackIcon
  return 'https://cos.lingkeai.vip/op.svg'
}

/**
 * Heuristic model classification by name, in priority order:
 * video -> image -> audio -> chat.
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
      'banana',
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
 * rate (ratio 1.0 = 100%).
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
  const official = ALL_MODELS_MAP.get(name.toLowerCase())
  const effectiveMultiplier = official?.group_multiplier ?? pricing?.model_ratio ?? 1.0
  const badgePercent = computeBadgePercent(effectiveMultiplier)
  const tone = badgePercent != null ? badgeToneFor(badgePercent) : 'cyan'

  const category = (official?.model_type as PlazaCategory) || classifyPlazaModel(name)
  const officialLogo = getOfficialLogo(name, official?.vendor, official?.icon || pricing?.icon)

  const tags = official?.tags || (
    effectiveMultiplier < 0.5
      ? ['特惠福利', `倍率 ${effectiveMultiplier}x`, '大厂直供']
      : ['官方直供', `倍率 ${effectiveMultiplier}x`, '高可用']
  )

  return {
    name,
    displayName: formatModelDisplayName(name, official?.display_name),
    category,
    description: official?.intro || pricing?.description || `${official?.vendor || '官方直供'} 核心大模型，支持高速多轮对话与流式调用。`,
    icon: officialLogo,
    vendorName: official?.vendor || pricing?.vendor_name || '大厂直供',
    modelType: (official?.model_type as PlazaModel['modelType']) || 'chat',
    billingMode: official?.billing_mode || (pricing?.quota_type === 1 ? '按次' : '按token'),
    priceMin: official?.price_min,
    outputPriceMin: official?.output_price_min,
    priceMax: official?.price_max,
    tags,
    successRate: official?.composite_success_rate ?? 100,
    fastestSeconds: official?.fastest_seconds,
    onlineLines: official?.online_lines ?? 5,
    protocols: official?.protocols,
    modelRatio: effectiveMultiplier,
    completionRatio: pricing?.completion_ratio,
    modelPrice: pricing?.model_price,
    quotaType: pricing?.quota_type,
    badgePercent,
    badgeTone: tone,
  }
}
