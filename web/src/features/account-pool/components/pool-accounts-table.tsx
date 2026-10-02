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
import type { ColumnDef, ColumnFiltersState, SortingState } from '@tanstack/react-table'
import {
  Download,
  ExternalLink,
  HeartPulse,
  Loader2,
  MoreHorizontal,
  Power,
  PowerOff,
  RefreshCcwDot,
  RefreshCw,
  Settings2,
  Trash2,
  Upload,
} from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  DataTableBulkActions,
  DataTablePage,
  useDataTable,
} from '@/components/data-table'
import { Dialog } from '@/components/dialog'
import { StatusBadge, type StatusVariant } from '@/components/status-badge'
import { TruncatedText } from '@/components/truncated-text'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Switch } from '@/components/ui/switch'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { ChannelTypeLogo } from '@/features/channels/components/channel-type-badge'
import {
  deleteChannel,
  refreshSubscriptionChannel,
  updateChannelStatus,
} from '@/features/channels/api'
import { CHANNEL_STATUS, CHANNEL_TYPES } from '@/features/channels/constants'
import { formatTimestampToDate } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { getServerErrorMessage } from '@/lib/server-error-message'
import { truncateText } from '@/lib/utils'

import {
  accountPoolQueryKeys,
  triggerPoolHealthTest,
  useAccountPool,
} from '../api'
import { PoolImportDialog } from './import-dialog'
import {
  getPoolCredential,
  POOL_STATUS,
  type PoolAccount,
} from '../types'

const POOL_STATUS_CONFIG: Record<number, { label: string; variant: StatusVariant }> = {
  [POOL_STATUS.ENABLED]: { label: 'Enabled', variant: 'success' },
  [POOL_STATUS.AUTO_DISABLED]: { label: 'Auto Disabled', variant: 'warning' },
  [POOL_STATUS.MANUALLY_DISABLED]: { label: 'Manually Disabled', variant: 'neutral' },
}

const POOL_STATUS_FALLBACK: { label: string; variant: StatusVariant } = {
  label: 'Unknown',
  variant: 'neutral',
}

const POOL_TYPE_LABEL_FALLBACK: Record<number, string> = {
  64: 'Antigravity',
}

const POOL_COLUMN_VISIBILITY_STORAGE_KEY = 'account-pool:column-visibility'

function getPoolTypeLabel(type: number): string {
  return (
    (CHANNEL_TYPES as Record<number, string>)[type] ??
    POOL_TYPE_LABEL_FALLBACK[type] ??
    'Unknown'
  )
}

function formatRfc3339(value?: string): string {
  if (!value) return ''
  const ms = Date.parse(value)
  if (Number.isNaN(ms)) return value
  return formatTimestampToDate(ms / 1000)
}

export type PoolAccountsTableProps = {
  onOpenSettings: () => void
}

export function PoolAccountsTable(props: PoolAccountsTableProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const poolQuery = useAccountPool()
  const accounts = poolQuery.data?.data?.accounts ?? []
  const groups = useMemo(
    () => [...new Set(accounts.map((a) => a.group).filter(Boolean))].sort(),
    [accounts]
  )
  const typeOptions = useMemo(() => {
    const ids = [...new Set(accounts.map((a) => a.type))].sort((a, b) => a - b)
    return ids.map((id) => ({ value: String(id), label: t(getPoolTypeLabel(id)) }))
  }, [accounts, t])

  const [sorting, setSorting] = useState<SortingState>([])
  const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([])
  const [globalFilter, setGlobalFilter] = useState('')
  const [importOpen, setImportOpen] = useState(false)
  const [autoRefresh, setAutoRefresh] = useState(false)
  const [pendingIds, setPendingIds] = useState<number[]>([])
  const [deleteTargets, setDeleteTargets] = useState<PoolAccount[] | null>(null)

  const invalidatePool = useCallback(() => {
    void queryClient.invalidateQueries({ queryKey: accountPoolQueryKeys.all })
  }, [queryClient])

  // Optional live refresh (5s) when the operator enabled auto refresh.
  useEffect(() => {
    if (!autoRefresh) return
    const timer = window.setInterval(invalidatePool, 5000)
    return () => window.clearInterval(timer)
  }, [autoRefresh, invalidatePool])

  const runWithPending = useCallback(
    async (ids: number[], task: (id: number) => Promise<void>) => {
      setPendingIds(ids)
      try {
        await Promise.all(ids.map(task))
      } finally {
        setPendingIds([])
        invalidatePool()
      }
    },
    [invalidatePool]
  )

  const handleHealthTest = useCallback(
    async (account: PoolAccount) => {
      setPendingIds([account.id])
      try {
        const response = await triggerPoolHealthTest({ channel_id: account.id })
        if (response.success) {
          toast.success(
            response.data?.ok ? t('Health check passed') : t('Health check failed')
          )
        } else {
          toast.error(getServerErrorMessage(response, t('Health check failed')))
        }
      } catch (error) {
        handleServerError(error, t('Health check failed'))
      } finally {
        setPendingIds([])
        invalidatePool()
      }
    },
    [t, invalidatePool]
  )

  const handleRefreshCredential = useCallback(
    async (account: PoolAccount) => {
      setPendingIds([account.id])
      try {
        const response = await refreshSubscriptionChannel(account.id)
        if (response.success) {
          toast.success(t('Credentials refreshed successfully'))
        } else {
          toast.error(
            getServerErrorMessage(response, t('Failed to refresh credentials'))
          )
        }
      } catch (error) {
        handleServerError(error, t('Failed to refresh credentials'))
      } finally {
        setPendingIds([])
        invalidatePool()
      }
    },
    [t, invalidatePool]
  )

  const handleToggleStatus = useCallback(
    async (account: PoolAccount) => {
      const isEnabled = account.status === POOL_STATUS.ENABLED
      const nextStatus = isEnabled
        ? CHANNEL_STATUS.MANUAL_DISABLED
        : CHANNEL_STATUS.ENABLED
      setPendingIds([account.id])
      try {
        const response = await updateChannelStatus(account.id, nextStatus)
        if (response.success) {
          toast.success(
            t(isEnabled ? 'Account disabled successfully' : 'Account enabled successfully')
          )
        } else {
          handleServerError(response, t('Failed to update account status'))
        }
      } catch (error) {
        handleServerError(error, t('Failed to update account status'))
      } finally {
        setPendingIds([])
        invalidatePool()
      }
    },
    [t, invalidatePool]
  )

  const handleDeleteRequest = useCallback((account: PoolAccount) => {
    setDeleteTargets([account])
  }, [])

  const confirmDelete = useCallback(async () => {
    const targets = deleteTargets
    if (!targets || targets.length === 0) return
    setDeleteTargets(null)
    await runWithPending(
      targets.map((a) => a.id),
      async (id) => {
        const response = await deleteChannel(id)
        if (response.success) {
          toast.success(t('Account deleted successfully'))
        } else {
          toast.error(getServerErrorMessage(response, t('Failed to delete account')))
        }
      }
    )
  }, [deleteTargets, runWithPending, t])

  const handleExport = useCallback(() => {
    const rows = accounts.map((a) => ({
      id: a.id,
      name: a.name,
      type: a.type,
      group: a.group,
      status: a.status,
      models: a.models,
      created_time: a.created_time,
    }))
    const blob = new Blob([JSON.stringify(rows, null, 2)], {
      type: 'application/json',
    })
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = `account-pool-${new Date().toISOString().slice(0, 10)}.json`
    link.click()
    // Keep the blob URL alive until the browser has started the download;
    // revoking it synchronously truncates the file in some engines.
    window.setTimeout(() => URL.revokeObjectURL(url), 30_000)
    toast.success(t('Accounts exported successfully'))
  }, [accounts, t])

  const columns = useMemo<ColumnDef<PoolAccount>[]>(
    () => [
      {
        id: 'select',
        header: ({ table }) => (
          <Checkbox
            checked={table.getIsAllPageRowsSelected()}
            indeterminate={table.getIsSomePageRowsSelected()}
            onCheckedChange={(value) =>
              table.toggleAllPageRowsSelected(!!value)
            }
            aria-label={t('Select all')}
          />
        ),
        cell: ({ row }) => (
          <Checkbox
            checked={row.getIsSelected()}
            onCheckedChange={(checked) => row.toggleSelected(checked === true)}
            aria-label={t('Select row')}
          />
        ),
        enableSorting: false,
        enableHiding: false,
        size: 36,
      },
      {
        id: 'name',
        accessorFn: (row: PoolAccount) => row.name,
        header: t('Name'),
        cell: ({ row }) => {
          const account = row.original
          return (
            <div className='flex min-w-0 items-center gap-2'>
              <span
                className='min-w-0 truncate font-medium'
                title={account.name}
              >
                {account.name}
              </span>
              <span className='text-muted-foreground shrink-0 text-xs tabular-nums'>
                #{account.id}
              </span>
            </div>
          )
        },
      },
      {
        id: 'type',
        accessorFn: (row: PoolAccount) => row.type,
        header: t('Platform / Type'),
        filterFn: (row, columnId, value) => {
          const selected = value as string[]
          if (!Array.isArray(selected) || selected.includes('all')) return true
          return selected.includes(String(row.getValue(columnId)))
        },
        cell: ({ row }) => {
          const account = row.original
          return (
            <div className='flex min-w-0 items-center gap-1.5'>
              <ChannelTypeLogo type={account.type} size={16} />
              <span className='truncate text-xs'>
                {t(getPoolTypeLabel(account.type))}
              </span>
            </div>
          )
        },
      },
      {
        id: 'account',
        accessorFn: (row: PoolAccount) => {
          const credential = getPoolCredential(row)
          return credential?.email || credential?.account_id || ''
        },
        header: t('Account'),
        cell: ({ row }) => {
          const credential = getPoolCredential(row.original)
          const identifier = credential?.email || credential?.account_id
          return identifier ? (
            <span
              className='text-muted-foreground block max-w-[180px] truncate font-mono text-xs'
              title={identifier}
            >
              {identifier}
            </span>
          ) : (
            <span className='text-muted-foreground text-xs'>-</span>
          )
        },
      },
      {
        id: 'capacity',
        header: t('Capacity'),
        cell: ({ row }) => {
          const runtime = row.original.pool_runtime
          const max = row.original.max_concurrency
          const active = runtime?.active_requests ?? 0
          const errorRate = runtime?.error_rate ?? 0
          return (
            <div className='flex min-w-0 flex-col items-start gap-0.5 text-xs'>
              <span className='tabular-nums'>
                {active}
                {max > 0 ? ` / ${max}` : ''} req
              </span>
              {runtime && runtime.total_requests > 0 ? (
                <span
                  className={
                    errorRate >= 0.5
                      ? 'text-destructive tabular-nums'
                      : 'text-muted-foreground tabular-nums'
                  }
                >
                  {t('Error {{percent}}%', {
                    percent: (errorRate * 100).toFixed(0),
                  })}
                </span>
              ) : null}
            </div>
          )
        },
      },
      {
        id: 'status',
        accessorFn: (row: PoolAccount) => row.status,
        header: t('Status'),
        filterFn: (row, _columnId, value) => {
          const selected = value as string[]
          if (!Array.isArray(selected) || selected.includes('all')) return true
          if (selected.includes('banned') && row.original.banned) return true
          return selected.includes(String(row.original.status))
        },
        cell: ({ row }) => {
          const account = row.original
          const config = POOL_STATUS_CONFIG[account.status] ?? POOL_STATUS_FALLBACK
          const nowSeconds = Math.floor(Date.now() / 1000)
          const inCooldown = (account.cooldown_deadline ?? 0) > nowSeconds
          const cooldownMinutes = inCooldown
            ? Math.max(1, Math.ceil((account.cooldown_deadline - nowSeconds) / 60))
            : 0

          return (
            <div className='flex min-w-0 flex-col items-start gap-1'>
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
              {account.health?.enabled ? (
                <StatusBadge
                  label={
                    account.health.results?.[0]
                      ? account.health.results[0].ok
                        ? t('Healthy')
                        : t('Failing')
                      : t('Health check every {{minutes}} min', {
                          minutes: account.health.interval_minutes || '?',
                        })
                  }
                  variant={
                    account.health.results?.[0]
                      ? account.health.results[0].ok
                        ? 'success'
                        : 'danger'
                      : 'info'
                  }
                  size='sm'
                  copyable={false}
                />
              ) : null}
              {account.status_reason ? (
                <Tooltip>
                  <TooltipTrigger
                    render={
                      <span className='text-muted-foreground max-w-[160px] cursor-help truncate text-xs' />
                    }
                  >
                    {truncateText(account.status_reason, 24)}
                  </TooltipTrigger>
                  <TooltipContent side='top' className='max-w-xs'>
                    {account.status_reason}
                  </TooltipContent>
                </Tooltip>
              ) : null}
            </div>
          )
        },
      },
      {
        id: 'schedule',
        header: t('Schedule'),
        cell: ({ row }) => {
          const account = row.original
          const isEnabled = account.status === POOL_STATUS.ENABLED
          const pending = pendingIds.includes(account.id)
          return (
            <Switch
              checked={isEnabled}
              disabled={pending}
              onCheckedChange={() => void handleToggleStatus(account)}
              aria-label={t('Schedule')}
            />
          )
        },
        enableSorting: false,
      },
      {
        id: 'group',
        accessorFn: (row: PoolAccount) => row.group,
        header: t('Group'),
        filterFn: (row, columnId, value) => {
          const selected = value as string[]
          if (!Array.isArray(selected) || selected.includes('all')) return true
          return selected.includes(String(row.getValue(columnId) ?? ''))
        },
        cell: ({ row }) => (
          <Badge
            variant='outline'
            className='max-w-[140px] truncate'
            title={row.original.group || undefined}
          >
            {row.original.group || '-'}
          </Badge>
        ),
      },
      {
        id: 'usage',
        header: t('Usage window'),
        cell: ({ row }) => {
          const runtime = row.original.pool_runtime
          const total = runtime?.total_requests ?? 0
          const active = runtime?.active_requests ?? 0
          return (
            <div className='flex min-w-0 flex-col items-start text-xs tabular-nums'>
              <span>{total} req</span>
              <span className='text-muted-foreground'>
                {t('Active {{count}}', { count: active })}
              </span>
            </div>
          )
        },
      },
      {
        id: 'credential',
        header: t('Credential'),
        cell: ({ row }) => {
          const credential = getPoolCredential(row.original)
          if (!credential) {
            return <span className='text-muted-foreground text-xs'>-</span>
          }
          const expiry = formatRfc3339(credential.expired)
          return (
            <div className='flex min-w-0 flex-col items-start gap-1'>
              <StatusBadge
                label={credential.valid ? t('Valid') : t('Expired')}
                variant={credential.valid ? 'success' : 'danger'}
                size='sm'
                copyable={false}
              />
              {expiry && (
                <span
                  className='text-muted-foreground max-w-[120px] truncate text-xs'
                  title={expiry}
                >
                  {expiry}
                </span>
              )}
            </div>
          )
        },
      },
      {
        id: 'models',
        header: t('Models'),
        cell: ({ row }) =>
          row.original.models ? (
            <TruncatedText
              text={row.original.models}
              maxWidth='max-w-[220px]'
              className='font-mono text-xs text-muted-foreground'
            />
          ) : (
            <span className='text-muted-foreground text-xs'>-</span>
          ),
      },
      {
        id: 'actions',
        header: () => <span className='block text-right'>{t('Actions')}</span>,
        cell: ({ row }) => {
          const account = row.original
          const credential = getPoolCredential(account)
          const isEnabled = account.status === POOL_STATUS.ENABLED
          const pending = pendingIds.includes(account.id)
          return (
            <div className='flex items-center justify-end'>
              <DropdownMenu>
                <DropdownMenuTrigger
                  render={
                    <Button
                      variant='ghost'
                      size='icon-sm'
                      aria-label={t('Actions')}
                      disabled={pending}
                    />
                  }
                >
                  {pending ? (
                    <Loader2 className='size-4 animate-spin' />
                  ) : (
                    <MoreHorizontal className='size-4' />
                  )}
                </DropdownMenuTrigger>
                <DropdownMenuContent align='end' className='w-44'>
                  <DropdownMenuItem
                    onSelect={() => void handleHealthTest(account)}
                  >
                    <HeartPulse />
                    {t('Health Check')}
                  </DropdownMenuItem>
                  {credential && (
                    <DropdownMenuItem
                      onSelect={() => void handleRefreshCredential(account)}
                    >
                      <RefreshCw />
                      {t('Refresh Credentials')}
                    </DropdownMenuItem>
                  )}
                  <DropdownMenuItem
                    onSelect={() => void handleToggleStatus(account)}
                  >
                    {isEnabled ? <PowerOff /> : <Power />}
                    {t(isEnabled ? 'Disable' : 'Enable')}
                  </DropdownMenuItem>
                  <DropdownMenuItem render={<Link to='/channels' />}>
                    <ExternalLink />
                    {t('Open Channels')}
                  </DropdownMenuItem>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem
                    variant='destructive'
                    onSelect={() => handleDeleteRequest(account)}
                  >
                    <Trash2 />
                    {t('Delete')}
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
          )
        },
        enableSorting: false,
      },
    ],
    [t, pendingIds, handleToggleStatus, handleHealthTest, handleRefreshCredential, handleDeleteRequest]
  )

  const { table } = useDataTable<PoolAccount>({
    data: accounts,
    columns,
    sorting,
    onSortingChange: setSorting,
    columnFilters,
    onColumnFiltersChange: setColumnFilters,
    globalFilter,
    onGlobalFilterChange: setGlobalFilter,
    initialColumnVisibility: { credential: false, models: false },
    columnVisibilityStorageKey: POOL_COLUMN_VISIBILITY_STORAGE_KEY,
    enableRowSelection: true,
    getRowId: (row) => String(row.id),
  })

  const selectedRows = table.getFilteredSelectedRowModel().rows
  const selectedIds = selectedRows.map((row) => row.original.id)

  const statusOptions = [
    { value: 'all', label: t('All statuses') },
    { value: String(POOL_STATUS.ENABLED), label: t('Enabled') },
    { value: String(POOL_STATUS.AUTO_DISABLED), label: t('Auto Disabled') },
    { value: String(POOL_STATUS.MANUALLY_DISABLED), label: t('Manually Disabled') },
    { value: 'banned', label: t('Banned') },
  ]

  const toolbarFilters = useMemo(
    () => [
      {
        columnId: 'type',
        title: t('Platform'),
        options: [
          { value: 'all', label: t('All platforms') },
          ...typeOptions,
        ],
        singleSelect: true,
      },
      {
        columnId: 'status',
        title: t('Status'),
        options: statusOptions,
        singleSelect: true,
      },
      {
        columnId: 'group',
        title: t('Group'),
        options: [
          { value: 'all', label: t('All groups') },
          ...groups.map((g) => ({ value: g, label: g })),
        ],
        singleSelect: true,
      },
    ],
    [t, typeOptions, groups]
  )

  return (
    <>
      <DataTablePage<PoolAccount>
        table={table}
        columns={columns}
        isLoading={poolQuery.isLoading}
        isFetching={poolQuery.isFetching}
        emptyTitle={t('No Accounts Found')}
        emptyDescription={t(
          'Channels with subscription credentials or API keys will appear here once they are created in Channel Management.'
        )}
        skeletonKeyPrefix='account-pool-skeleton'
        toolbarProps={{
          collapsibleOnMobile: true,
          searchPlaceholder: t('Search accounts...'),
          searchDebounceMs: 300,
          filters: toolbarFilters,
          preActions: (
            <>
              <DropdownMenu>
                <DropdownMenuTrigger
                  render={
                    <Button variant='outline' size='sm' className='gap-1.5' />
                  }
                >
                  <MoreHorizontal className='size-4' />
                  {t('More actions')}
                </DropdownMenuTrigger>
                <DropdownMenuContent align='end' className='w-52'>
                  <DropdownMenuLabel>{t('Data operations')}</DropdownMenuLabel>
                  <DropdownMenuItem onSelect={() => setImportOpen(true)}>
                    <Upload />
                    {t('Import accounts')}
                  </DropdownMenuItem>
                  <DropdownMenuItem onSelect={handleExport}>
                    <Download />
                    {t('Export accounts')}
                  </DropdownMenuItem>
                  <DropdownMenuSeparator />
                  <DropdownMenuLabel>{t('Tools')}</DropdownMenuLabel>
                  <DropdownMenuItem onSelect={() => invalidatePool()}>
                    <RefreshCcwDot />
                    {t('Refresh now')}
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    onSelect={() =>
                      void runWithPending(
                        accounts.map((a) => a.id),
                        async (id) => {
                          const account = accounts.find((a) => a.id === id)
                          if (account) await handleHealthTest(account)
                        }
                      )
                    }
                  >
                    <HeartPulse />
                    {t('Batch health check')}
                  </DropdownMenuItem>
                  <DropdownMenuItem onSelect={props.onOpenSettings}>
                    <Settings2 />
                    {t('Account pool settings')}
                  </DropdownMenuItem>
                  <DropdownMenuSeparator />
                  <DropdownMenuLabel>{t('Display')}</DropdownMenuLabel>
                  <DropdownMenuItem
                    onSelect={() => setAutoRefresh((previous) => !previous)}
                  >
                    <RefreshCcwDot />
                    {t('Auto refresh')}
                    {autoRefresh && (
                      <Badge variant='secondary' className='ms-auto text-xs'>
                        ON
                      </Badge>
                    )}
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
              <Button size='sm' onClick={() => setImportOpen(true)}>
                <Upload />
                {t('Add account')}
              </Button>
            </>
          ),
        }}
        bulkActions={
          <DataTableBulkActions table={table} entityName='account'>
            <Tooltip>
              <TooltipTrigger
                render={
                  <Button
                    variant='outline'
                    size='icon'
                    className='size-8'
                    aria-label={t('Enable selected accounts')}
                    onClick={() =>
                      void runWithPending(selectedIds, async (id) => {
                        await updateChannelStatus(id, CHANNEL_STATUS.ENABLED)
                      })
                    }
                  />
                }
              >
                <Power />
                <span className='sr-only'>{t('Enable selected accounts')}</span>
              </TooltipTrigger>
              <TooltipContent>
                <p>{t('Enable selected accounts')}</p>
              </TooltipContent>
            </Tooltip>
            <Tooltip>
              <TooltipTrigger
                render={
                  <Button
                    variant='outline'
                    size='icon'
                    className='size-8'
                    aria-label={t('Disable selected accounts')}
                    onClick={() =>
                      void runWithPending(selectedIds, async (id) => {
                        await updateChannelStatus(
                          id,
                          CHANNEL_STATUS.MANUAL_DISABLED
                        )
                      })
                    }
                  />
                }
              >
                <PowerOff />
                <span className='sr-only'>{t('Disable selected accounts')}</span>
              </TooltipTrigger>
              <TooltipContent>
                <p>{t('Disable selected accounts')}</p>
              </TooltipContent>
            </Tooltip>
            <Tooltip>
              <TooltipTrigger
                render={
                  <Button
                    variant='outline'
                    size='icon'
                    className='size-8'
                    aria-label={t('Health check selected accounts')}
                    onClick={() =>
                      void runWithPending(selectedIds, async (id) => {
                        const account = accounts.find((a) => a.id === id)
                        if (account) await handleHealthTest(account)
                      })
                    }
                  />
                }
              >
                <HeartPulse />
                <span className='sr-only'>
                  {t('Health check selected accounts')}
                </span>
              </TooltipTrigger>
              <TooltipContent>
                <p>{t('Health check selected accounts')}</p>
              </TooltipContent>
            </Tooltip>
            <Tooltip>
              <TooltipTrigger
                render={
                  <Button
                    variant='destructive'
                    size='icon'
                    className='size-8'
                    aria-label={t('Delete selected accounts')}
                    onClick={() =>
                      setDeleteTargets(selectedRows.map((row) => row.original))
                    }
                  />
                }
              >
                <Trash2 />
                <span className='sr-only'>{t('Delete selected accounts')}</span>
              </TooltipTrigger>
              <TooltipContent>
                <p>{t('Delete selected accounts')}</p>
              </TooltipContent>
            </Tooltip>
          </DataTableBulkActions>
        }
      />

      <PoolImportDialog
        open={importOpen}
        onOpenChange={setImportOpen}
        onImported={invalidatePool}
      />

      <Dialog
        open={deleteTargets !== null && deleteTargets.length > 0}
        onOpenChange={(open) => {
          if (!open) setDeleteTargets(null)
        }}
        title={t('Delete Accounts?')}
        description={
          <>
            {t('Are you sure you want to delete')}{' '}
            {deleteTargets?.length ?? 0}{' '}
            {t('account(s)? This action cannot be undone.')}
          </>
        }
        contentHeight='auto'
        footer={
          <>
            <Button
              variant='outline'
              onClick={() => setDeleteTargets(null)}
            >
              {t('Cancel')}
            </Button>
            <Button variant='destructive' onClick={() => void confirmDelete()}>
              {t('Delete')}
            </Button>
          </>
        }
      >
        <div />
      </Dialog>
    </>
  )
}
