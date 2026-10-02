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
import { useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import {
  AlertTriangle,
  Ban,
  Boxes,
  CheckCircle2,
  Download,
  ExternalLink,
  Loader2,
  Power,
  PowerOff,
  RefreshCw,
  ShieldAlert,
  Timer,
  type LucideIcon,
} from 'lucide-react'
import {
  useCallback,
  useMemo,
  useState,
  type ReactNode,
} from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { SectionPageLayout } from '@/components/layout'
import {
  StaticDataTable,
  type StaticDataTableColumn,
} from '@/components/data-table'
import { StatusBadge, type StatusVariant } from '@/components/status-badge'
import { TruncatedText } from '@/components/truncated-text'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from '@/components/ui/empty'
import { IconBadge, type IconBadgeTone } from '@/components/ui/icon-badge'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Separator } from '@/components/ui/separator'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { refreshSubscriptionChannel, updateChannelStatus } from '@/features/channels/api'
import { CHANNEL_STATUS, CHANNEL_TYPES } from '@/features/channels/constants'
import { formatTimestampToDate } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { getServerErrorMessage } from '@/lib/server-error-message'
import { truncateText } from '@/lib/utils'

import {
  accountPoolQueryKeys,
  useAccountPool,
  useUpdateAccountPoolSettings,
} from './api'
import { PoolImportDialog } from './components/import-dialog'
import {
  getPoolCredential,
  POOL_STATUS,
  type PoolAccount,
  type PoolSettings,
  type PoolStats,
} from './types'

type TranslateFn = (key: string, options?: Record<string, unknown>) => string

// Display names for pool channel types that are not covered by CHANNEL_TYPES.
const POOL_TYPE_LABEL_FALLBACK: Record<number, string> = {
  64: 'Antigravity',
}

function getPoolTypeLabel(type: number): string {
  const known = CHANNEL_TYPES[type as keyof typeof CHANNEL_TYPES]
  if (known) return known
  return POOL_TYPE_LABEL_FALLBACK[type] ?? 'Unknown'
}

const POOL_STATUS_CONFIG: Record<number, { label: string; variant: StatusVariant }> = {
  [POOL_STATUS.ENABLED]: { label: 'Enabled', variant: 'success' },
  [POOL_STATUS.AUTO_DISABLED]: { label: 'Auto Disabled', variant: 'warning' },
  [POOL_STATUS.MANUALLY_DISABLED]: { label: 'Manually Disabled', variant: 'neutral' },
}

const POOL_STATUS_FALLBACK: { label: string; variant: StatusVariant } = {
  label: 'Unknown',
  variant: 'neutral',
}

function formatRfc3339(value?: string): string {
  if (!value) return ''
  const ms = Date.parse(value)
  if (Number.isNaN(ms)) return value
  return formatTimestampToDate(ms / 1000)
}

function formatRemainingSeconds(totalSeconds: number, t: TranslateFn): string {
  if (!Number.isFinite(totalSeconds) || totalSeconds <= 0) return ''
  const days = Math.floor(totalSeconds / 86400)
  const hours = Math.floor((totalSeconds % 86400) / 3600)
  return t('{{days}} days, {{hours}} hours left', { days, hours })
}

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

function PoolAccountTable() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const poolQuery = useAccountPool()
  const accounts = poolQuery.data?.data?.accounts ?? []
  const isLoading = poolQuery.isLoading
  const isError = poolQuery.isError
  const [refreshingId, setRefreshingId] = useState<number | null>(null)
  const [togglingId, setTogglingId] = useState<number | null>(null)

  const invalidatePool = useCallback(() => {
    void queryClient.invalidateQueries({ queryKey: accountPoolQueryKeys.all })
  }, [queryClient])

  const handleRefreshCredential = useCallback(
    async (account: PoolAccount) => {
      setRefreshingId(account.id)
      try {
        const response = await refreshSubscriptionChannel(account.id)
        if (response.success) {
          toast.success(t('Credentials refreshed successfully'))
          invalidatePool()
        } else {
          toast.error(
            getServerErrorMessage(response, t('Failed to refresh credentials'))
          )
        }
      } catch (error) {
        handleServerError(error, t('Failed to refresh credentials'))
      } finally {
        setRefreshingId(null)
      }
    },
    [t, invalidatePool]
  )

  const handleToggleStatus = useCallback(
    async (account: PoolAccount) => {
      const isEnabled = account.status === POOL_STATUS.ENABLED
      // Reuse the channel status endpoint: enabling always targets the
      // enabled state, manual disabling targets the manual-disabled state.
      const nextStatus = isEnabled
        ? CHANNEL_STATUS.MANUAL_DISABLED
        : CHANNEL_STATUS.ENABLED
      setTogglingId(account.id)
      try {
        const response = await updateChannelStatus(account.id, nextStatus)
        if (response.success) {
          toast.success(
            t(
              isEnabled
                ? 'Account disabled successfully'
                : 'Account enabled successfully'
            )
          )
          invalidatePool()
        } else {
          handleServerError(response, t('Failed to update account status'))
        }
      } catch (error) {
        handleServerError(error, t('Failed to update account status'))
      } finally {
        setTogglingId(null)
      }
    },
    [t, invalidatePool]
  )

  const columns = useMemo<StaticDataTableColumn<PoolAccount>[]>(
    () => [
      // Name (with channel type badge)
      {
        id: 'name',
        header: t('Name'),
        cell: (account) => (
          <div className='flex min-w-0 items-center gap-2'>
            <span
              className='min-w-0 truncate font-medium'
              title={account.name}
            >
              {account.name}
            </span>
            <Badge
              variant='outline'
              className='min-w-0 max-w-[190px] truncate'
              title={t(getPoolTypeLabel(account.type))}
            >
              {t(getPoolTypeLabel(account.type))}
            </Badge>
          </div>
        ),
      },
      // Email / account identifier
      {
        id: 'account',
        header: t('Account'),
        cell: (account) => {
          const credential = getPoolCredential(account)
          const identifier = credential?.email || credential?.account_id
          return identifier ? (
            <span
              className='text-muted-foreground block max-w-[190px] truncate font-mono text-xs'
              title={identifier}
            >
              {identifier}
            </span>
          ) : (
            <span className='text-muted-foreground text-xs'>-</span>
          )
        },
      },
      // Credential validity / expiration / remaining time
      {
        id: 'credential',
        header: t('Credential'),
        cell: (account) => {
          const credential = getPoolCredential(account)
          if (!credential) {
            return <span className='text-muted-foreground text-xs'>-</span>
          }
          const expiry = formatRfc3339(credential.expired)
          const remaining = formatRemainingSeconds(
            credential.remaining_seconds,
            t
          )
          return (
            <div className='flex min-w-0 flex-col items-start gap-1'>
              <StatusBadge
                label={credential.valid ? t('Valid') : t('Expired')}
                variant={credential.valid ? 'success' : 'danger'}
                size='sm'
                copyable={false}
              />
              {(expiry || remaining) && (
                <span
                  className='text-muted-foreground flex max-w-full items-center gap-1 text-xs'
                  title={expiry || undefined}
                >
                  {expiry && <span className='min-w-0 truncate'>{expiry}</span>}
                  {remaining && <span className='shrink-0'>· {remaining}</span>}
                </span>
              )}
            </div>
          )
        },
      },
      // Status (enabled / auto disabled / manually disabled / banned / cooldown)
      {
        id: 'status',
        header: t('Status'),
        cell: (account) => {
          const config =
            POOL_STATUS_CONFIG[account.status] ?? POOL_STATUS_FALLBACK
          const nowSeconds = Math.floor(Date.now() / 1000)
          const inCooldown = (account.cooldown_deadline ?? 0) > nowSeconds
          const cooldownMinutes = inCooldown
            ? Math.max(
                1,
                Math.ceil((account.cooldown_deadline - nowSeconds) / 60)
              )
            : 0

          return (
            <div className='flex min-w-0 flex-col items-start gap-1'>
              <div className='flex min-w-0 flex-wrap items-center gap-1'>
                <StatusBadge
                  label={t(config.label)}
                  variant={config.variant}
                  size='sm'
                  copyable={false}
                />
                {account.banned && (
                  <StatusBadge
                    label={t('Banned')}
                    variant='danger'
                    size='sm'
                    copyable={false}
                  />
                )}
              </div>
              {inCooldown && (
                <StatusBadge
                  label={t('Cooling down, {{minutes}} min left', {
                    minutes: cooldownMinutes,
                  })}
                  variant='info'
                  size='sm'
                  copyable={false}
                />
              )}
              {account.status_reason ? (
                <TooltipProvider delay={100}>
                  <Tooltip>
                    <TooltipTrigger
                      render={
                        <span className='text-muted-foreground max-w-[200px] cursor-help truncate text-xs' />
                      }
                    >
                      {truncateText(account.status_reason, 32)}
                    </TooltipTrigger>
                    <TooltipContent side='top' className='max-w-xs'>
                      {account.status_reason}
                    </TooltipContent>
                  </Tooltip>
                </TooltipProvider>
              ) : null}
            </div>
          )
        },
      },
      // Models (truncated)
      {
        id: 'models',
        header: t('Models'),
        cell: (account) =>
          account.models ? (
            <TruncatedText
              text={account.models}
              maxWidth='max-w-[240px]'
              className='font-mono text-xs text-muted-foreground'
            />
          ) : (
            <span className='text-muted-foreground text-xs'>-</span>
          ),
      },
      // Group
      {
        id: 'group',
        header: t('Group'),
        cell: (account) => (
          <span
            className='text-muted-foreground block max-w-[140px] truncate text-xs'
            title={account.group || undefined}
          >
            {account.group || '-'}
          </span>
        ),
      },
      // Actions: refresh credential / toggle status / open channel management
      {
        id: 'actions',
        header: <span className='block text-right'>{t('Actions')}</span>,
        cellClassName: 'text-right',
        cell: (account) => {
          const credential = getPoolCredential(account)
          const isEnabled = account.status === POOL_STATUS.ENABLED
          const isRefreshing = refreshingId === account.id
          const isToggling = togglingId === account.id

          let toggleIcon: ReactNode
          if (isToggling) {
            toggleIcon = <Loader2 className='size-4 animate-spin' />
          } else if (isEnabled) {
            toggleIcon = <PowerOff className='size-4' />
          } else {
            toggleIcon = <Power className='size-4' />
          }

          return (
            <div className='flex items-center justify-end gap-0.5'>
              {credential && (
                <Tooltip>
                  <TooltipTrigger
                    render={
                      <Button
                        variant='ghost'
                        size='icon-sm'
                        aria-label={t('Refresh Credentials')}
                        disabled={isRefreshing}
                        onClick={() => void handleRefreshCredential(account)}
                      />
                    }
                  >
                    {isRefreshing ? (
                      <Loader2 className='size-4 animate-spin' />
                    ) : (
                      <RefreshCw className='size-4' />
                    )}
                  </TooltipTrigger>
                  <TooltipContent>{t('Refresh Credentials')}</TooltipContent>
                </Tooltip>
              )}
              <Tooltip>
                <TooltipTrigger
                  render={
                    <Button
                      variant='ghost'
                      size='icon-sm'
                      aria-label={isEnabled ? t('Disable') : t('Enable')}
                      disabled={isToggling}
                      onClick={() => void handleToggleStatus(account)}
                    />
                  }
                >
                  {toggleIcon}
                </TooltipTrigger>
                <TooltipContent>
                  {isEnabled ? t('Disable') : t('Enable')}
                </TooltipContent>
              </Tooltip>
              <Tooltip>
                <TooltipTrigger
                  render={
                    <Button
                      variant='ghost'
                      size='icon-sm'
                      aria-label={t('Open Channels')}
                      render={<Link to='/channels' />}
                    />
                  }
                >
                  <ExternalLink className='size-4' />
                </TooltipTrigger>
                <TooltipContent>{t('Open Channels')}</TooltipContent>
              </Tooltip>
            </div>
          )
        },
      },
    ],
    [t, refreshingId, togglingId, handleRefreshCredential, handleToggleStatus]
  )

  if (isError) {
    return (
      <div className='flex shrink-0 flex-col items-center gap-3 rounded-md border py-12 text-center'>
        <AlertTriangle className='text-muted-foreground size-6' />
        <p className='text-muted-foreground text-sm'>
          {t('Failed to load the account pool. Please try again.')}
        </p>
        <Button variant='outline' size='sm' onClick={() => poolQuery.refetch()}>
          {t('Retry')}
        </Button>
      </div>
    )
  }

  return (
    <div className='shrink-0'>
      <StaticDataTable
        tableClassName='min-w-[960px]'
        columns={columns}
        data={isLoading ? [] : accounts}
        getRowKey={(row) => row.id}
        empty={isLoading || accounts.length === 0}
        emptyContent={
          isLoading ? (
            <div className='flex flex-col gap-3 py-3'>
              {Array.from({ length: 5 }, (_, index) => (
                <div key={index} className='flex items-center gap-3'>
                  <Skeleton className='h-5 w-[22%]' />
                  <Skeleton className='h-5 w-[16%]' />
                  <Skeleton className='h-5 w-[18%]' />
                  <Skeleton className='h-5 w-[16%]' />
                  <Skeleton className='h-5 w-[12%]' />
                </div>
              ))}
            </div>
          ) : (
            <Empty className='border-0 py-12'>
              <EmptyHeader>
                <EmptyTitle>{t('No Accounts Found')}</EmptyTitle>
                <EmptyDescription>
                  {t(
                    'Channels with subscription credentials will appear here once they are created in Channel Management.'
                  )}
                </EmptyDescription>
              </EmptyHeader>
            </Empty>
          )
        }
      />
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
  const queryClient = useQueryClient()
  const [importOpen, setImportOpen] = useState(false)

  return (
    <SectionPageLayout fixedContent>
      <SectionPageLayout.Title>{t('Account Pool')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='flex h-full min-h-0 flex-col gap-4 overflow-auto pr-0.5'>
          <div className='flex shrink-0 items-start justify-between gap-2'>
            <p className='text-muted-foreground text-sm'>
              {t(
                'Monitor subscription accounts in the pool, refresh credentials and configure anti-ban protection.'
              )}
            </p>
            <Button
              size='sm'
              variant='outline'
              onClick={() => setImportOpen(true)}
            >
              <Download />
              {t('Import accounts')}
            </Button>
          </div>
          <PoolStatsCards />
          <PoolAccountTable />
          <PoolSettingsCard />
          <PoolImportDialog
            open={importOpen}
            onOpenChange={setImportOpen}
            onImported={() =>
              void queryClient.invalidateQueries({
                queryKey: accountPoolQueryKeys.all,
              })
            }
          />
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}