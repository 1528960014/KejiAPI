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

import type { PlazaBadgeTone, PlazaCategory, PlazaModel } from '../types'

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
  const badgePercent = computeBadgePercent(pricing?.model_ratio)
  const tone = badgePercent != null ? badgeToneFor(badgePercent) : 'cyan'

  return {
    name,
    category: classifyPlazaModel(name),
    description: pricing?.description,
    icon: pricing?.icon,
    vendorName: pricing?.vendor_name,
    modelRatio: pricing?.model_ratio,
    completionRatio: pricing?.completion_ratio,
    modelPrice: pricing?.model_price,
    quotaType: pricing?.quota_type,
    badgePercent,
    badgeTone: tone,
  }
}
