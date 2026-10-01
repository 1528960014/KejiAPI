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
import { api } from '@/lib/api'
import { requireServerSuccess } from '@/lib/server-error-message'

import type { PricingData } from '@/features/pricing/types'
import type { TaskLog } from '@/features/usage-logs/types'

import {
  ALL_OFFICIAL_MODELS,
  buildPlazaModel,
  SUB_KEJIKE_MODELS,
} from './models'
import type { PlazaModel } from '../types'

interface UserModelsResponse {
  success: boolean
  message?: string
  data?: string[]
}

/**
 * Load the user's model list (`GET /api/user/models`) and enrich each
 * name with pricing data (`GET /api/pricing`) for descriptions, icons
 * and ratio-based badge percentages. Each endpoint is independent: a
 * failed pricing lookup still yields a usable model list with 100%
 * badges, and an empty user list falls back to the pricing catalog.
 */
export async function fetchPlazaModels(): Promise<PlazaModel[]> {
  const [modelsResult, pricingResult] = await Promise.allSettled([
    api.get<UserModelsResponse>('/api/user/models'),
    api.get<PricingData>('/api/pricing'),
  ])

  const userModels: string[] = []
  if (modelsResult.status === 'fulfilled') {
    const payload = requireServerSuccess(modelsResult.value.data)
    if (payload?.success && Array.isArray(payload.data)) {
      userModels.push(...payload.data)
    }
  }

  const pricingByName = new Map<string, PricingData['data'][number]>()
  if (pricingResult.status === 'fulfilled') {
    const payload = requireServerSuccess(pricingResult.value.data)
    if (payload?.success && Array.isArray(payload.data)) {
      for (const entry of payload.data) {
        if (entry?.model_name) {
          pricingByName.set(entry.model_name.toLowerCase(), entry)
        }
      }
    }
  }

  const sourceNames = userModels.length > 0 ? userModels : Array.from(pricingByName.keys())

  const seen = new Set<string>()
  const models: PlazaModel[] = []
  for (const name of sourceNames) {
    if (!name || seen.has(name)) continue
    seen.add(name)
    models.push(buildPlazaModel(name, pricingByName.get(name.toLowerCase())))
  }
  return models
}

export type { TaskLog as PlazaTaskLog }

/**
 * Recent task history for the signed-in user
 * (`GET /api/task/self?p=1&page_size=20`), same endpoint shape as the
 * usage-logs task view.
 */
export async function fetchPlazaTaskLogs(
  pageSize = 20
): Promise<TaskLog[]> {
  const res = await api.get<{
    success: boolean
    message?: string
    data?: { items: TaskLog[]; total: number }
  }>(`/api/task/self?p=1&page_size=${pageSize}`)
  const payload = requireServerSuccess(res.data)
  if (!payload?.success || !payload.data?.items) return []
  return payload.data.items
}
