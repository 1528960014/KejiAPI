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
  Loader2,
  PowerOff,
  ShieldAlert,
  Timer,
  type LucideIcon,
} from 'lucide-react'
import { useCallback, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { IconBadge, type IconBadgeTone } from '@/components/ui/icon-badge'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Separator } from '@/components/ui/separator'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import { handleServerError } from '@/lib/handle-server-error'

import { useAccountPool, useUpdateAccountPoolSettings } from './api'
import { PoolAccountsTable } from './components/pool-accounts-table'
import type { PoolSettings, PoolStats } from './types'

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
    <div className='grid shrink-0 grid-cols-2 gap-3 sm:grid-cols-3 xl:grid-cols-5'>
      {cards.map((card) => {
        const Icon = card.icon
        const value = card.getValue(stats)
        return (
          <Card key={card.key} size='sm' className='gap-1.5'>
            <div className='flex items-center gap-2 px-4'>
              <IconBadge tone={card.tone} size='sm' className='shrink-0'>
                <Icon />
              </IconBadge>
              <span className='text-muted-foreground truncate text-xs font-medium'>
                {card.label}
              </span>
            </div>
            <div className='px-4 pb-1 text-xl font-semibold tracking-tight tabular-nums'>
              {isLoading || value === undefined ? (
                <Skeleton className='h-6 w-10' />
              ) : (
                value
              )}
            </div>
          </Card>
        )
      })}
    </div>
  )
}

function PoolSettingsCard() {
  const { t } = useTranslation()
  const poolQuery = useAccountPool()
  const serverSettings = poolQuery.data?.data?.settings
  const updateSettings = useUpdateAccountPoolSettings()
  const [form, setForm] = useState<PoolSettings | null>(null)
  const [syncedSettings, setSyncedSettings] = useState<PoolSettings | null>(
    null
  )

  // Reset the draft when the server state changes (initial load + post-save).
  // Adjusting state during render (instead of an effect) keeps the form in
  // sync without cascading renders.
  if (serverSettings && serverSettings !== syncedSettings) {
    setSyncedSettings(serverSettings)
    setForm(serverSettings)
  }

  const isDirty =
    form !== null &&
    serverSettings !== null &&
    JSON.stringify(form) !== JSON.stringify(serverSettings)

  const updateField = <K extends keyof PoolSettings>(
    key: K,
    value: PoolSettings[K]
  ) => {
    setForm((previous) =>
      previous ? { ...previous, [key]: value } : previous
    )
  }

  const handleSave = () => {
    if (!form) return
    updateSettings.mutate(form, {
      onSuccess: () => toast.success(t('Settings saved successfully')),
      onError: (error) => handleServerError(error, t('Failed to save settings')),
    })
  }

  return (
    <Card className='shrink-0'>
      <CardHeader>
        <CardTitle className='flex items-center gap-2'>
          <ShieldAlert className='text-muted-foreground size-4' />
          {t('Account Pool Management')}
        </CardTitle>
        <CardDescription>
          {t(
            'Scheduling, session stickiness, cooldowns, isolation and rate limiting for pooled accounts.'
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className='grid gap-5'>
        <div className='flex flex-wrap items-center justify-between gap-3'>
          <div className='min-w-0'>
            <Label>{t('Cooldown Protection')}</Label>
            <p className='text-muted-foreground mt-1 text-xs'>
              {t(
                'Temporarily hold auto-disabled accounts out of rotation for a fixed duration.'
              )}
            </p>
          </div>
          <div className='flex items-center gap-2'>
            {form?.cooldown_enabled ? (
              <div className='flex items-center gap-1.5'>
                <Input
                  type='number'
                  min={0}
                  step={1}
                  className='w-20'
                  value={form.cooldown_minutes}
                  onChange={(event) =>
                    updateField(
                      'cooldown_minutes',
                      Math.max(0, Number(event.target.value) || 0)
                    )
                  }
                />
                <span className='text-muted-foreground text-sm'>
                  {t('minutes')}
                </span>
              </div>
            ) : null}
            <Switch
              checked={form?.cooldown_enabled ?? false}
              onCheckedChange={(checked) =>
                updateField('cooldown_enabled', checked === true)
              }
              disabled={!form}
              aria-label={t('Cooldown Protection')}
            />
          </div>
        </div>

        <Separator />

        <div className='flex flex-wrap items-center justify-between gap-3'>
          <div className='min-w-0'>
            <Label>{t('Rate Limit Cooldown')}</Label>
            <p className='text-muted-foreground mt-1 text-xs'>
              {t(
                'Cooldown applied when the upstream answers 429. Rate-limit windows are short, so this stays in seconds.'
              )}
            </p>
          </div>
          <div className='flex items-center gap-2'>
            <Input
              type='number'
              min={5}
              step={1}
              className='w-24'
              value={form?.rate_limit_cooldown_seconds ?? 0}
              onChange={(event) =>
                updateField(
                  'rate_limit_cooldown_seconds',
                  Math.max(5, Number(event.target.value) || 0)
                )
              }
            />
            <span className='text-muted-foreground text-sm'>
              {t('seconds')}
            </span>
          </div>
        </div>

        <Separator />

        <div className='flex flex-wrap items-center justify-between gap-3'>
          <div className='min-w-0'>
            <Label>{t('Overload Cooldown')}</Label>
            <p className='text-muted-foreground mt-1 text-xs'>
              {t(
                'Isolation time applied when the upstream answers 529 (overloaded).'
              )}
            </p>
          </div>
          <div className='flex items-center gap-2'>
            <Input
              type='number'
              min={1}
              step={1}
              className='w-20'
              value={form?.overload_cooldown_minutes ?? 0}
              onChange={(event) =>
                updateField(
                  'overload_cooldown_minutes',
                  Math.max(1, Number(event.target.value) || 0)
                )
              }
            />
            <span className='text-muted-foreground text-sm'>
              {t('minutes')}
            </span>
          </div>
        </div>

        <Separator />

        <div className='flex flex-wrap items-center justify-between gap-3'>
          <div className='min-w-0'>
            <Label>{t('Credential Cooldown')}</Label>
            <p className='text-muted-foreground mt-1 text-xs'>
              {t(
                'Recovery time for temporary credential errors (401 / 403 without an explicit ban).'
              )}
            </p>
          </div>
          <div className='flex items-center gap-2'>
            <Input
              type='number'
              min={1}
              step={1}
              className='w-20'
              value={form?.credential_cooldown_minutes ?? 0}
              onChange={(event) =>
                updateField(
                  'credential_cooldown_minutes',
                  Math.max(1, Number(event.target.value) || 0)
                )
              }
            />
            <span className='text-muted-foreground text-sm'>
              {t('minutes')}
            </span>
          </div>
        </div>

        <Separator />

        <div className='flex flex-wrap items-center justify-between gap-3'>
          <div className='min-w-0'>
            <Label>{t('Selection Depth')}</Label>
            <p className='text-muted-foreground mt-1 text-xs'>
              {t(
                'How many of the healthiest pool accounts take part in each weighted draw. Lower concentrates traffic on healthy accounts; 0 disables load-aware ordering.'
              )}
            </p>
          </div>
          <Input
            type='number'
            min={0}
            max={32}
            step={1}
            className='w-20'
            value={form?.selection_top_k ?? 0}
            onChange={(event) =>
              updateField(
                'selection_top_k',
                Math.min(32, Math.max(0, Number(event.target.value) || 0))
              )
            }
          />
        </div>

        <Separator />

        <div className='flex flex-wrap items-center justify-between gap-3'>
          <div className='min-w-0'>
            <Label>{t('Error Escape Rate')}</Label>
            <p className='text-muted-foreground mt-1 text-xs'>
              {t(
                'Recent-error rate at which a sticky account stops being preferred and traffic moves to another pooled account.'
              )}
            </p>
          </div>
          <div className='flex items-center gap-2'>
            <Input
              type='number'
              min={0.1}
              max={1}
              step={0.05}
              className='w-24'
              value={form?.escape_error_rate ?? 0.5}
              onChange={(event) =>
                updateField(
                  'escape_error_rate',
                  Math.min(1, Math.max(0.1, Number(event.target.value) || 0.5))
                )
              }
            />
          </div>
        </div>

        <Separator />

        <div className='flex flex-wrap items-center justify-between gap-3'>
          <div className='min-w-0'>
            <Label>{t('Health Check Interval')}</Label>
            <p className='text-muted-foreground mt-1 text-xs'>
              {t(
                'Default interval of the per-account scheduled health checks. Enable them per account in channel settings.'
              )}
            </p>
          </div>
          <div className='flex items-center gap-2'>
            <Input
              type='number'
              min={5}
              max={1440}
              step={1}
              className='w-20'
              value={form?.health_check_default_interval_minutes ?? 60}
              onChange={(event) =>
                updateField(
                  'health_check_default_interval_minutes',
                  Math.min(
                    1440,
                    Math.max(5, Number(event.target.value) || 60)
                  )
                )
              }
            />
            <span className='text-muted-foreground text-sm'>
              {t('minutes')}
            </span>
          </div>
        </div>

        <Separator />

        <div className='flex flex-wrap items-center justify-between gap-3'>
          <div className='min-w-0'>
            <Label>{t('Ban Isolation')}</Label>
            <p className='text-muted-foreground mt-1 text-xs'>
              {t(
                'Keep banned accounts isolated so they are never selected for new requests.'
              )}
            </p>
          </div>
          <Switch
            checked={form?.ban_isolate_enabled ?? false}
            onCheckedChange={(checked) =>
              updateField('ban_isolate_enabled', checked === true)
            }
            disabled={!form}
            aria-label={t('Ban Isolation')}
          />
        </div>

        <Separator />

        <div className='flex flex-wrap items-center justify-between gap-3'>
          <div className='min-w-0'>
            <Label>{t('Rate Limiting')}</Label>
            <p className='text-muted-foreground mt-1 text-xs'>
              {t(
                'Limit how many requests each account can serve inside a rolling window.'
              )}
            </p>
          </div>
          <div className='flex items-center gap-2'>
            {form?.rate_limit_enabled ? (
              <>
                <div className='flex items-center gap-1.5'>
                  <Input
                    type='number'
                    min={1}
                    step={1}
                    className='w-24'
                    value={form.rate_limit_requests}
                    onChange={(event) =>
                      updateField(
                        'rate_limit_requests',
                        Math.max(1, Number(event.target.value) || 1)
                      )
                    }
                  />
                  <span className='text-muted-foreground text-sm'>
                    {t('requests')}
                  </span>
                </div>
                <div className='flex items-center gap-1.5'>
                  <Input
                    type='number'
                    min={1}
                    step={1}
                    className='w-20'
                    value={form.rate_limit_window_minutes}
                    onChange={(event) =>
                      updateField(
                        'rate_limit_window_minutes',
                        Math.max(1, Number(event.target.value) || 1)
                      )
                    }
                  />
                  <span className='text-muted-foreground text-sm'>
                    {t('minutes')}
                  </span>
                </div>
              </>
            ) : null}
            <Switch
              checked={form?.rate_limit_enabled ?? false}
              onCheckedChange={(checked) =>
                updateField('rate_limit_enabled', checked === true)
              }
              disabled={!form}
              aria-label={t('Rate Limiting')}
            />
          </div>
        </div>

        <Separator />

        <div className='flex flex-wrap items-center justify-between gap-3'>
          <div className='min-w-0'>
            <Label>{t('Session Stickiness')}</Label>
            <p className='text-muted-foreground mt-1 text-xs'>
              {t(
                'Keep each API key on the same pooled account per model and group, so multi-turn sessions stay on one account.'
              )}
            </p>
          </div>
          <Switch
            checked={form?.session_stickiness_enabled ?? false}
            onCheckedChange={(checked) =>
              updateField('session_stickiness_enabled', checked === true)
            }
            disabled={!form}
            aria-label={t('Session Stickiness')}
          />
        </div>
      </CardContent>
      <CardFooter className='justify-end'>
        <Button
          onClick={handleSave}
          disabled={!form || !isDirty || updateSettings.isPending}
        >
          {updateSettings.isPending ? (
            <Loader2 className='size-4 animate-spin' />
          ) : null}
          {t('Save')}
        </Button>
      </CardFooter>
    </Card>
  )
}

export function AccountPool() {
  const { t } = useTranslation()
  const settingsRef = useRef<HTMLDivElement>(null)

  const handleOpenSettings = useCallback(() => {
    settingsRef.current?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  }, [])

  return (
    <SectionPageLayout fixedContent>
      <SectionPageLayout.Title>{t('Account Management')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='flex h-full min-h-0 flex-col gap-4 overflow-auto pr-0.5'>
          <p className='text-muted-foreground shrink-0 text-sm'>
            {t(
              'Manage AI platform accounts and credentials: schedule, health and anti-ban protection.'
            )}
          </p>
          <PoolStatsCards />
          <PoolAccountsTable onOpenSettings={handleOpenSettings} />
          <div ref={settingsRef}>
            <PoolSettingsCard />
          </div>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
