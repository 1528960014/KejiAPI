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
  Ban,
  Boxes,
  CheckCircle2,
  PowerOff,
  Timer,
  type LucideIcon,
} from 'lucide-react'
import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import {
  Card,
} from '@/components/ui/card'
import { IconBadge, type IconBadgeTone } from '@/components/ui/icon-badge'
import { Skeleton } from '@/components/ui/skeleton'

import { useAccountPool } from './api'
import { PoolAccountsTable } from './components/pool-accounts-table'
import { PoolSettingsDialog } from './components/pool-settings-dialog'
import type { PoolStats } from './types'

function PoolStatsCards() {
  const { t } = useTranslation()
  const poolQuery = useAccountPool()
  const stats = poolQuery.data?.data?.stats
  const isLoading = poolQuery.isLoading

  const cards: {
    key: string
    label: string
    icon: LucideIcon
    tone: IconBadgeTone
    getValue: (s?: PoolStats) => number | undefined
  }[] = [
    {
      key: 'total',
      label: t('Total'),
      icon: Boxes,
      tone: 'primary',
      getValue: (s) => s?.total,
    },
    {
      key: 'enabled',
      label: t('Enabled'),
      icon: CheckCircle2,
      tone: 'success',
      getValue: (s) => s?.by_status.enabled,
    },
    {
      key: 'cooldown',
      label: t('In Cooldown'),
      icon: Timer,
      tone: 'info',
      getValue: (s) => s?.cooldown,
    },
    {
      key: 'banned',
      label: t('Banned'),
      icon: Ban,
      tone: 'destructive',
      getValue: (s) => s?.banned,
    },
    {
      key: 'manually_disabled',
      label: t('Manually Disabled'),
      icon: PowerOff,
      tone: 'neutral',
      getValue: (s) => s?.by_status.manually_disabled,
    },
  ]

  return (
    <div className='grid shrink-0 grid-cols-2 gap-2 sm:grid-cols-3 xl:grid-cols-5'>
      {cards.map((card) => {
        const Icon = card.icon
        const value = card.getValue(stats)
        return (
          <Card key={card.key} size='sm' className='gap-1 py-2'>
            <div className='flex items-center gap-2 px-3'>
              <IconBadge tone={card.tone} size='sm' className='shrink-0'>
                <Icon />
              </IconBadge>
              <div className='min-w-0 flex-1'>
                <p className='text-muted-foreground truncate text-xs'>
                  {card.label}
                </p>
                {isLoading ? (
                  <Skeleton className='mt-1 h-5 w-10' />
                ) : (
                  <p className='text-sm font-semibold tracking-tight'>
                    {value ?? 0}
                  </p>
                )}
              </div>
            </div>
          </Card>
        )
      })}
    </div>
  )
}

export function AccountPool() {
  const { t } = useTranslation()
  const [settingsOpen, setSettingsOpen] = useState(false)

  const handleOpenSettings = useCallback(() => {
    setSettingsOpen(true)
  }, [])

  return (
    <SectionPageLayout fixedContent>
      <SectionPageLayout.Title>
        <span className='flex min-w-0 items-center gap-2'>
          <span className='truncate'>{t('Account Management')}</span>
        </span>
      </SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='flex h-full min-h-0 flex-col gap-2.5'>
          <PoolStatsCards />
          <div className='min-h-0 flex-1 overflow-hidden'>
            <PoolAccountsTable onOpenSettings={handleOpenSettings} />
          </div>
        </div>
      </SectionPageLayout.Content>
      <PoolSettingsDialog open={settingsOpen} onOpenChange={setSettingsOpen} />
    </SectionPageLayout>
  )
}
