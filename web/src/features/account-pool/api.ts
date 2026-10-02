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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { api } from '@/lib/api'
import { requireServerSuccess } from '@/lib/server-error-message'

import type {
  AccountPoolImportResponse,
  AccountPoolResponse,
  AccountPoolSettingsResponse,
  PoolSettings,
} from './types'

export const accountPoolQueryKeys = {
  all: ['account-pool'] as const,
  detail: () => [...accountPoolQueryKeys.all, 'detail'] as const,
}

/**
 * Get the full account pool: accounts, stats and anti-ban settings.
 */
export async function getAccountPool(): Promise<AccountPoolResponse> {
  const res = await api.get<AccountPoolResponse>('/api/channel/pool')
  return requireServerSuccess(res.data)
}

/**
 * Update the anti-ban policy settings. Returns the persisted settings.
 */
export async function saveAccountPoolSettings(
  settings: PoolSettings
): Promise<PoolSettings> {
  const res = await api.put<AccountPoolSettingsResponse>(
    '/api/channel/pool/settings',
    settings
  )
  return requireServerSuccess(res.data).data
}

/**
 * Bulk-import subscription accounts from pasted credential data.
 * `dry_run` parses without creating channels (preview).
 * `max_concurrency` sets a per-account concurrency ceiling (0 = unlimited);
 * `expires_days` auto-pauses imported accounts after N days (0 = never).
 */
export async function importAccountPool(params: {
  raw: string
  group?: string
  dry_run?: boolean
  max_concurrency?: number
  expires_days?: number
}): Promise<AccountPoolImportResponse> {
  const res = await api.post<AccountPoolImportResponse>(
    '/api/channel/pool/import',
    params
  )
  return requireServerSuccess(res.data)
}

/**
 * Run one on-demand health check for a pool account.
 */
export async function triggerPoolHealthTest(params: {
  channel_id: number
  model?: string
}): Promise<{ success: boolean; message?: string; data?: { ok: boolean; latency_ms: number; error: string } }> {
  const res = await api.post('/api/channel/pool/health-test', params)
  return requireServerSuccess(res.data)
}

/**
 * Account pool query. Refetches periodically so cooldown deadlines and
 * credential remaining time stay fresh.
 */
export function useAccountPool() {
  return useQuery({
    queryKey: accountPoolQueryKeys.detail(),
    queryFn: getAccountPool,
    retry: false,
    refetchInterval: 30 * 1000,
  })
}

/**
 * Mutation that persists the anti-ban policy settings and refreshes the pool.
 */
export function useUpdateAccountPoolSettings() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: saveAccountPoolSettings,
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: accountPoolQueryKeys.all,
      })
    },
  })
}