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
  ALL_OFFICIAL_MODELS,
  formatModelDisplayName,
  getOfficialLogo,
} from '@/features/chat-plaza/lib/models'

import type { PricingModel } from '../types'

// ----------------------------------------------------------------------------
// Built-in official catalog pricing
// ----------------------------------------------------------------------------
// The model square ships an offline official catalog so every listed model
// always shows a price, even before any upstream channel has been configured.
// These entries are display-only; billing still follows the backend pricing
// records. Conversions:
//   - model_ratio = 1 corresponds to USD 2 / 1M tokens (see formatPrice).
//   - completion_ratio is the output/input price multiple.
//   - per-request models (image / video / audio) use model_price in USD.

const USD_PER_M_PER_RATIO = 2

/** Protocol label -> supported endpoint type key. */
const PROTOCOL_ENDPOINT_MAP: Record<string, string> = {
  openai: 'openai',
  anthropic: 'anthropic',
  claude: 'anthropic',
  gemini: 'gemini',
  google: 'gemini',
  azure: 'openai',
}

const BUILTIN_ENDPOINTS = ['openai', 'anthropic', 'gemini']

function toEndpoints(protocols?: string[]): string[] {
  if (!Array.isArray(protocols) || protocols.length === 0) return BUILTIN_ENDPOINTS
  const mapped = protocols
    .map((p) => PROTOCOL_ENDPOINT_MAP[String(p).toLowerCase()])
    .filter((v): v is string => Boolean(v))
  return mapped.length > 0 ? [...new Set(mapped)] : BUILTIN_ENDPOINTS
}

/** Default output/input multiple used when the catalog lacks an output price. */
const DEFAULT_COMPLETION_MULTIPLE = 3

function roundTo(value: number, digits: number): number {
  const factor = 10 ** digits
  return Math.round(value * factor) / factor
}

/**
 * Convert one catalog entry into a display-only pricing record.
 * Returns undefined when the catalog has no usable price.
 */
function toPricingModel(meta: {
  model_name: string
  display_name?: string
  intro?: string
  vendor?: string
  model_type?: 'chat' | 'image' | 'video' | 'audio'
  protocols?: string[]
  price_min?: number
  output_price_min?: number
  icon?: string
}): PricingModel | undefined {
  const priceIn = meta.price_min
  if (typeof priceIn !== 'number' || !Number.isFinite(priceIn) || priceIn <= 0) {
    return undefined
  }
  const isPerRequest = (meta.model_type ?? 'chat') !== 'chat'
  const icon = getOfficialLogo(meta.model_name, meta.vendor, meta.icon)

  if (isPerRequest) {
    return {
      id: 0,
      model_name: meta.model_name,
      description: meta.intro,
      icon,
      vendor_name: meta.vendor || '大厂直供',
      vendor_icon: icon,
      quota_type: 1,
      model_ratio: 0,
      completion_ratio: 0,
      model_price: priceIn,
      enable_groups: ['all'],
      supported_endpoint_types: toEndpoints(meta.protocols),
    }
  }

  const priceOut =
    typeof meta.output_price_min === 'number' &&
    Number.isFinite(meta.output_price_min) &&
    meta.output_price_min > 0
      ? meta.output_price_min
      : priceIn * DEFAULT_COMPLETION_MULTIPLE

  return {
    id: 0,
    model_name: meta.model_name,
    description: meta.intro,
    icon,
    vendor_name: meta.vendor || '大厂直供',
    vendor_icon: icon,
    quota_type: 0,
    model_ratio: roundTo(priceIn / USD_PER_M_PER_RATIO, 6),
    completion_ratio: roundTo(priceOut / priceIn, 4),
    enable_groups: ['all'],
    supported_endpoint_types: toEndpoints(meta.protocols),
  }
}

let cachedOfficialModels: PricingModel[] | undefined

/** Display-only pricing records for the whole built-in official catalog. */
export function getOfficialBuiltinModels(): PricingModel[] {
  if (cachedOfficialModels) return cachedOfficialModels
  const list: PricingModel[] = []
  const seen = new Set<string>()
  for (const meta of ALL_OFFICIAL_MODELS) {
    if (!meta?.model_name) continue
    const lower = meta.model_name.toLowerCase()
    if (seen.has(lower)) continue
    seen.add(lower)
    const pricing = toPricingModel(meta)
    if (pricing) list.push({ ...pricing, key: meta.model_name })
  }
  cachedOfficialModels = list
  return list
}

/** Display name helper reused by consumers of this module. */
export function officialDisplayName(name: string): string {
  return formatModelDisplayName(name)
}

/**
 * Merge built-in catalog pricing into backend pricing records.
 *
 * - Models present in the backend keep their configured values.
 * - Backend models without a configured price inherit the official price.
 * - Catalog models missing from the backend are appended as read-only entries.
 */
export function mergeOfficialBuiltinPricing(
  models: PricingModel[]
): PricingModel[] {
  const officialByName = new Map(getOfficialBuiltinModels().map((m) => [m.model_name.toLowerCase(), m]))

  const merged = models.map((model) => {
    const official = officialByName.get(model.model_name.toLowerCase())
    if (!official) return model
    const hasConfiguredPrice =
      model.quota_type === 1
        ? typeof model.model_price === 'number' && Number.isFinite(model.model_price) && model.model_price > 0
        : typeof model.model_ratio === 'number' && Number.isFinite(model.model_ratio) && model.model_ratio > 0
    if (hasConfiguredPrice) return model
    return {
      ...model,
      description: model.description || official.description,
      icon: model.icon || official.icon,
      vendor_name: model.vendor_name || official.vendor_name,
      vendor_icon: model.vendor_icon || official.vendor_icon,
      ...(model.quota_type === official.quota_type
        ? {
            quota_type: official.quota_type,
            model_ratio: official.model_ratio,
            completion_ratio: official.completion_ratio,
            model_price: official.model_price,
          }
        : {}),
      enable_groups:
        Array.isArray(model.enable_groups) && model.enable_groups.length > 0
          ? model.enable_groups
          : official.enable_groups,
      supported_endpoint_types:
        model.supported_endpoint_types && model.supported_endpoint_types.length > 0
          ? model.supported_endpoint_types
          : official.supported_endpoint_types,
    }
  })

  for (const official of officialByName.values()) {
    if (!models.some((m) => m.model_name.toLowerCase() === official.model_name.toLowerCase())) {
      merged.push(official)
    }
  }
  return merged
}
