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
import type { PlazaModel } from '../types'

/**
 * Rough client-side cost estimate for the "预计 ⚡x/秒" chip.
 *
 * It treats generation as a constant token throughput (video-style
 * workloads consume tokens continuously while the clip renders) and
 * converts quota to "credits" (1 credit = 1,000 quota). This is only
 * a display heuristic — actual billing always comes from the server.
 */
const INPUT_TOKENS_PER_SECOND = 800
const OUTPUT_TOKENS_PER_SECOND = 1600
const QUOTA_PER_TOKEN = 50
const QUOTA_PER_CREDIT = 1000

export function estimateCreditsPerSecond(
  model: PlazaModel | null
): number {
  const ratio = model?.modelRatio ?? 1
  const completionRatio = model?.completionRatio ?? 1
  const quotaPerSecond =
    (INPUT_TOKENS_PER_SECOND * ratio +
      OUTPUT_TOKENS_PER_SECOND * ratio * completionRatio) *
    QUOTA_PER_TOKEN
  return quotaPerSecond / QUOTA_PER_CREDIT
}

export function formatCredits(value: number): string {
  if (!Number.isFinite(value) || value <= 0) return '0'
  return value >= 100 ? String(Math.round(value)) : value.toFixed(1)
}
