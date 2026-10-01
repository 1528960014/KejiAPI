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
// Chat Plaza (对话广场) feature types.

export type PlazaCategory = 'all' | 'chat' | 'image' | 'video' | 'audio'

export type PlazaBadgeTone = 'green' | 'cyan' | 'orange'

/**
 * A model merged from `/api/user/models` (names) and `/api/pricing`
 * (descriptions / ratios), plus client-side classification.
 */
export interface PlazaModel {
  name: string
  category: PlazaCategory
  displayName?: string
  description?: string
  icon?: string
  vendorName?: string
  modelType?: 'chat' | 'image' | 'video' | 'audio'
  billingMode?: '按token' | '按次' | '按秒' | string
  priceMin?: number
  outputPriceMin?: number
  priceMax?: number
  tags?: string[]
  successRate?: number
  fastestSeconds?: number
  onlineLines?: number
  protocols?: string[]
  modelRatio?: number
  completionRatio?: number
  modelPrice?: number
  quotaType?: number
  /** Percentage badge; undefined means the pricing data was unavailable
   *  and the UI falls back to a neutral 100% badge. */
  badgePercent?: number
  badgeTone: PlazaBadgeTone
}

export interface PlazaSettings {
  quality: 'best' | 'high' | 'fast'
  count: 1 | 2 | 4
  mode: 'text' | 'firstLast' | 'reference'
  duration: 5 | 10 | 15 | 30
  ratio: 'auto' | '16:9' | '9:16' | '1:1'
  resolution: '720p' | '1080p'
  watermark: 'off' | 'on'
}

export const DEFAULT_PLAZA_SETTINGS: PlazaSettings = {
  quality: 'best',
  count: 1,
  mode: 'reference',
  duration: 5,
  ratio: 'auto',
  resolution: '720p',
  watermark: 'off',
}

export type PlazaMessageStatus = 'streaming' | 'complete' | 'error'

export interface PlazaMessage {
  id: string
  role: 'user' | 'assistant'
  content: string
  status: PlazaMessageStatus
  error?: string
}

/** Locally-selected reference file; previews only, never uploaded. */
export interface PlazaRefFile {
  file: File
  /** Object URL for image previews; undefined for video/audio. */
  previewUrl?: string
}

export interface PlazaRefSelection {
  image?: PlazaRefFile | null
  video?: PlazaRefFile | null
  audio?: PlazaRefFile | null
}
