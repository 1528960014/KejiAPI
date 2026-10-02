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
// ============================================================================
// Account Pool (GET /api/channel/pool)
//
// Pool account status values are the channel status values:
// 1=enabled, 2=manually disabled, 3=auto disabled
// (identical to CHANNEL_STATUS in features/channels/constants.ts).
// ============================================================================

export const POOL_STATUS = {
  ENABLED: 1,
  MANUALLY_DISABLED: 2,
  AUTO_DISABLED: 3,
} as const

export type PoolAccountStatus = (typeof POOL_STATUS)[keyof typeof POOL_STATUS]

/**
 * Subscription credential view attached to pool accounts.
 * Channels without subscription credentials return an empty object.
 */
export type PoolCredential = {
  provider: string
  email: string
  account_id: string
  /** RFC3339 expiration timestamp */
  expired: string
  /** RFC3339 last refresh timestamp */
  last_refresh: string
  remaining_seconds: number
  valid: boolean
  auto_refresh: boolean
}

export type PoolAccount = {
  id: number
  name: string
  type: number
  /** 1=enabled, 2=manually_disabled, 3=auto_disabled (see POOL_STATUS) */
  status: PoolAccountStatus
  status_reason: string
  models: string
  group: string
  priority: number | null
  weight: number
  auto_ban: boolean
  created_time: number
  /** Unix seconds deadline; 0 means no active cooldown */
  cooldown_deadline: number
  banned: boolean
  credential: PoolCredential | {}
}

export type PoolStats = {
  total: number
  by_type: Record<number, number>
  by_status: {
    enabled: number
    auto_disabled: number
    manually_disabled: number
  }
  cooldown: number
  banned: number
}

/** Anti-ban policy settings persisted by PUT /api/channel/pool/settings */
export type PoolSettings = {
  cooldown_enabled: boolean
  cooldown_minutes: number
  ban_isolate_enabled: boolean
  rate_limit_enabled: boolean
  rate_limit_requests: number
  rate_limit_window_minutes: number
}

export type AccountPoolData = {
  accounts: PoolAccount[]
  stats: PoolStats
  settings: PoolSettings
}

export type AccountPoolResponse = {
  success: boolean
  message?: string
  data: AccountPoolData
}

export type AccountPoolSettingsResponse = {
  success: boolean
  message?: string
  data: PoolSettings
}

/**
 * Returns the credential view of a pool account, or null when the account
 * has no subscription credential (empty object).
 */
export function getPoolCredential(
  account: PoolAccount
): PoolCredential | null {
  const credential = account.credential
  if (Object.keys(credential).length === 0) {
    return null
  }
  // A non-empty credential object always carries the full credential view.
  return credential as PoolCredential
}

// ============================================================================
// Pool import (POST /api/channel/pool/import)
// ============================================================================

/** One parsed entry reported by the import endpoint. */
export type PoolImportResultItem = {
  index: number
  provider: string
  email: string
  name?: string
  channel_id?: number
  error?: string
}

export type AccountPoolImportResponse = {
  success: boolean
  message?: string
  data: {
    total: number
    created: number
    dry_run: boolean
    results: PoolImportResultItem[]
  }
}