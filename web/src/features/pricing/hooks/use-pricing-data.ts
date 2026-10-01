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
import { useQuery } from '@tanstack/react-query'
import { useMemo } from 'react'

import {
  ALL_OFFICIAL_MODELS,
  ALL_MODELS_MAP,
  getOfficialLogo,
  formatModelDisplayName,
} from '@/features/chat-plaza/lib/models'
import { useStatus } from '@/hooks/use-status'
import { requireServerSuccess } from '@/lib/server-error-message'

import { getPricing } from '../api'
import type { PricingModel, PricingVendor } from '../types'

const PRESET_VENDORS: Array<{ name: string; icon: string; description: string }> = [
  { name: 'Anthropic', icon: 'https://cos.lingkeai.vip/claude.svg', description: 'Anthropic Claude 系列官方模型' },
  { name: 'OpenAI', icon: 'https://cos.lingkeai.vip/op.svg', description: 'OpenAI GPT / o1 / o3 官方模型' },
  { name: 'Google', icon: 'https://cos.lingkeai.vip/gem.svg', description: 'Google Gemini 官方大模型' },
  { name: 'DeepSeek', icon: 'https://cos.lingkeai.vip/deepseek.svg', description: 'DeepSeek R1 / V3 / V4 满血版' },
  { name: '月之暗面', icon: 'https://cos.lingkeai.vip/kimi.svg', description: 'Moonshot Kimi 长文本大模型' },
  { name: '智谱AI', icon: 'https://cos.lingkeai.vip/glm.svg', description: '智谱 GLM 新一代核心大模型' },
  { name: '阿里百炼', icon: 'https://cos.lingkeai.vip/qwen.svg', description: '通义千问 Qwen 系列' },
  { name: '快手', icon: 'https://cos.lingkeai.vip/kling.svg', description: '可灵 Kling 视频生成大模型' },
  { name: '字节跳动', icon: 'https://cos.lingkeai.vip/doubao.svg', description: '豆包 / 即梦 / Seedance 系列' },
  { name: 'MiniMax', icon: 'https://cos.lingkeai.vip/minimax.svg', description: '海螺 AI / MiniMax 语言与多模态模型' },
  { name: '阶跃星辰', icon: 'https://cos.lingkeai.vip/step.svg', description: 'Step 系列万亿参数大模型' },
  { name: 'xAI', icon: 'https://cos.lingkeai.vip/op.svg', description: '马斯克 xAI Grok 系列' },
  { name: '大厂直供', icon: 'https://cos.lingkeai.vip/op.svg', description: '专线高可用聚合通道' },
]

export function usePricingData(enabled = true) {
  const { status } = useStatus()

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['pricing'],
    queryFn: async () => requireServerSuccess(await getPricing()),
    staleTime: 5 * 60 * 1000,
    enabled,
  })

  // Ensure rates never reach zero to prevent division errors
  const priceRate = useMemo(
    () => Math.max((status?.price as number) ?? 1, 0.001),
    [status?.price]
  )
  const usdExchangeRate = useMemo(
    () => Math.max((status?.usd_exchange_rate as number) ?? priceRate, 0.001),
    [status?.usd_exchange_rate, priceRate]
  )

  const { models, vendors, groupRatio, usableGroup } = useMemo(() => {
    const rawVendors: PricingVendor[] = [...(data?.vendors ?? [])]
    const vendorMap = new Map<number, PricingVendor>(rawVendors.map((v) => [v.id, v]))
    const vendorByName = new Map<string, PricingVendor>(
      rawVendors.map((v) => [v.name.toLowerCase(), v])
    )

    // Ensure all preset vendors are included
    let nextVendorId = rawVendors.length > 0 ? Math.max(...rawVendors.map((v) => v.id)) + 1 : 100
    for (const pv of PRESET_VENDORS) {
      if (!vendorByName.has(pv.name.toLowerCase())) {
        const newVendor: PricingVendor = {
          id: nextVendorId++,
          name: pv.name,
          icon: pv.icon,
          description: pv.description,
        }
        rawVendors.push(newVendor)
        vendorByName.set(pv.name.toLowerCase(), newVendor)
        vendorMap.set(newVendor.id, newVendor)
      }
    }

    const mergedGroupRatio: Record<string, number> = { ...(data?.group_ratio ?? {}) }
    const mergedUsableGroup: Record<string, { desc: string; ratio: number }> = {
      ...(data?.usable_group ?? {}),
    }

    const list: PricingModel[] = []
    const seen = new Set<string>()

    // 1. Process backend models if present
    if (Array.isArray(data?.data)) {
      for (const m of data.data) {
        if (!m?.model_name) continue
        const lower = m.model_name.toLowerCase()
        seen.add(lower)
        const vendor = m.vendor_id ? vendorMap.get(m.vendor_id) : undefined
        const official = ALL_MODELS_MAP.get(lower)
        const vName = vendor?.name || official?.vendor || '大厂直供'
        const officialIcon = getOfficialLogo(m.model_name, vName, m.icon || vendor?.icon)

        list.push({
          ...m,
          key: m.model_name,
          description: m.description || official?.intro,
          icon: officialIcon,
          vendor_name: vName,
          vendor_icon: officialIcon,
          vendor_description: vendor?.description || official?.intro,
          group_ratio: data?.group_ratio,
        })
      }
    }

    // 2. Synthesize official catalog models (151 sub.kejike + 174 LK888)
    let nextSyntheticId = 10000
    for (const official of ALL_OFFICIAL_MODELS) {
      const lower = official.model_name.toLowerCase()
      if (seen.has(lower)) continue
      seen.add(lower)

      const vName = official.vendor || '大厂直供'
      const matchedVendor = vendorByName.get(vName.toLowerCase())
      const officialIcon = getOfficialLogo(official.model_name, vName, official.icon)
      const groupName = official.group_name || 'default'
      const multiplier = official.group_multiplier ?? 1.0

      if (groupName && !mergedGroupRatio[groupName]) {
        mergedGroupRatio[groupName] = multiplier
        mergedUsableGroup[groupName] = {
          desc: `${groupName} (倍率 ${multiplier}x)`,
          ratio: multiplier,
        }
      }

      const isPerRequest = official.billing_mode === '按次'
      const modelPrice = isPerRequest ? (official.price_min ?? 0.1) : 0
      const completionRatio =
        official.output_price_min && official.price_min && official.price_min > 0
          ? Number((official.output_price_min / official.price_min).toFixed(3))
          : 2.0

      list.push({
        id: nextSyntheticId++,
        key: official.model_name,
        model_name: official.model_name,
        description: official.intro || `${vName} 官方核心模型，支持高速多轮对话与流式调用。`,
        icon: officialIcon,
        vendor_id: matchedVendor?.id,
        vendor_name: vName,
        vendor_icon: officialIcon,
        vendor_description: matchedVendor?.description,
        quota_type: isPerRequest ? 1 : 0,
        model_ratio: multiplier,
        completion_ratio: completionRatio,
        model_price: modelPrice,
        enable_groups: [groupName],
        tags: (official.tags || []).join(','),
        group_ratio: mergedGroupRatio,
      })
    }

    return {
      models: list,
      vendors: rawVendors,
      groupRatio: mergedGroupRatio,
      usableGroup: mergedUsableGroup,
    }
  }, [data])

  return {
    models,
    vendors,
    groupRatio,
    usableGroup,
    endpointMap: data?.supported_endpoint ?? {},
    autoGroups: data?.auto_groups ?? [],
    isLoading,
    error,
    refetch,
    priceRate,
    usdExchangeRate,
  }
}
