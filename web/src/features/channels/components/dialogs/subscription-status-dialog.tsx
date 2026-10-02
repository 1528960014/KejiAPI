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
import { AlertTriangle, Check, Copy, Loader2, RefreshCw } from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { StatusBadge, type StatusBadgeProps } from '@/components/status-badge'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import dayjs from '@/lib/dayjs'
import { formatDateTimeStr, formatTimestampToDate } from '@/lib/format'
import {
  createServerError,
  getServerErrorMessage,
} from '@/lib/server-error-message'
import { cn } from '@/lib/utils'

import {
  getSubscriptionChannelStatus,
  refreshSubscriptionChannel,
  type SubscriptionCredentialStatusData,
} from '../../api'

type SubscriptionStatusDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  channelName?: string
  channelId?: number
}

function formatCredentialTime(value: unknown): string {
  if (value == null || value === '') {
    return '-'
  }
  const text = String(value).trim()
  if (!text) {
    return '-'
  }
  if (/^\d+(\.\d+)?$/.test(text)) {
    const seconds = Number(text)
    return Number.isFinite(seconds) && seconds > 0
      ? formatTimestampToDate(seconds)
      : '-'
  }
  const parsed = dayjs(text)
  return parsed.isValid() ? formatDateTimeStr(parsed.toDate()) : text
}

function formatRemainingSeconds(
  value: unknown,
  t: (key: string) => string
): string {
  const s = Number(value)
  if (!Number.isFinite(s)) {
    return '-'
  }
  if (s <= 0) {
    return t('Expired')
  }
  const total = Math.floor(s)
  const hours = Math.floor(total / 3600)
  const minutes = Math.floor((total % 3600) / 60)
  const secs = total % 60
  if (hours > 0) {
    return `${hours}${t('h')} ${minutes}${t('m')}`
  }
  if (minutes > 0) {
    return `${minutes}${t('m')} ${secs}${t('s')}`
  }
  return `${secs}${t('s')}`
}

function InfoField(props: {
  label: string
  value?: string | number | null
  mono?: boolean
  copyable?: boolean
  className?: string
}) {
  const { t } = useTranslation()
  const { copyToClipboard, copiedText } = useCopyToClipboard({ notify: false })
  const text = props.value == null ? '' : String(props.value)
  const trimmed = text.trim()
  const hasCopied = trimmed !== '' && copiedText === trimmed

  return (
    <div
      className={cn(
        'bg-background ring-border/60 min-w-0 rounded-lg p-3 ring-1',
        props.className
      )}
    >
      <div className='text-muted-foreground text-[11px] font-medium'>
        {props.label}
      </div>
      <div className='mt-1 flex min-w-0 items-start justify-between gap-2'>
        <span
          className={cn(
            'min-w-0 flex-1 text-xs leading-5 break-all',
            props.mono && 'font-mono tabular-nums'
          )}
        >
          {trimmed || '-'}
        </span>
        {props.copyable !== false && trimmed ? (
          <Button
            type='button'
            variant='ghost'
            size='icon-xs'
            aria-label={t('Copy')}
            onClick={() => void copyToClipboard(trimmed)}
          >
            {hasCopied ? (
              <Check className='text-success' />
            ) : (
              <Copy />
            )}
          </Button>
        ) : null}
      </div>
    </div>
  )
}

export function SubscriptionStatusDialog({
  open,
  onOpenChange,
  channelName,
  channelId,
}: SubscriptionStatusDialogProps) {
  const { t } = useTranslation()
  const [status, setStatus] = useState<SubscriptionCredentialStatusData | null>(
    null
  )
  const [isLoading, setIsLoading] = useState(false)
  const [isRefreshing, setIsRefreshing] = useState(false)
  const [error, setError] = useState('')

  const fetchStatus = useCallback(async () => {
    if (!channelId) {
      return
    }
    setIsLoading(true)
    setError('')
    try {
      const res = await getSubscriptionChannelStatus(channelId)
      if (!res.success) {
        throw createServerError(res, t('Failed to fetch credential status'))
      }
      setStatus(res.data ?? null)
    } catch (err) {
      setError(
        getServerErrorMessage(err, t('Failed to fetch credential status'))
      )
    } finally {
      setIsLoading(false)
    }
  }, [channelId, t])

  useEffect(() => {
    if (!open || !channelId) {
      return
    }
    void fetchStatus()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, channelId])

  const handleOpenChange = (nextOpen: boolean) => {
    if (!nextOpen) {
      setStatus(null)
      setError('')
      setIsRefreshing(false)
    }
    onOpenChange(nextOpen)
  }

  const handleRefresh = async () => {
    if (!channelId || isRefreshing) {
      return
    }
    setIsRefreshing(true)
    setError('')
    try {
      const res = await refreshSubscriptionChannel(channelId)
      if (!res.success) {
        throw createServerError(res, t('Failed to refresh credentials'))
      }
      setStatus(res.data ?? null)
      toast.success(t('Credentials refreshed'))
    } catch (err) {
      setError(getServerErrorMessage(err, t('Failed to refresh credentials')))
    } finally {
      setIsRefreshing(false)
    }
  }

  const channelLabel = channelName
    ? `${channelName}${channelId ? ` (#${channelId})` : ''}`
    : ''
  const validVariant: StatusBadgeProps['variant'] =
    status && status.valid === false ? 'danger' : 'success'

  return (
    <Dialog
      open={open}
      onOpenChange={handleOpenChange}
      title={t('Subscription Credentials')}
      description={t('Credential Status')}
      contentHeight='auto'
      bodyClassName='flex flex-col gap-4'
      footer={
        <>
          <Button
            type='button'
            variant='outline'
            onClick={() => void handleRefresh()}
            disabled={!channelId || isLoading || isRefreshing}
          >
            {isRefreshing ? (
              <Loader2 data-icon='inline-start' className='animate-spin' />
            ) : (
              <RefreshCw data-icon='inline-start' />
            )}
            {isRefreshing ? t('Refreshing...') : t('Refresh Credentials')}
          </Button>
          <Button
            type='button'
            variant='ghost'
            onClick={() => handleOpenChange(false)}
          >
            {t('Close')}
          </Button>
        </>
      }
    >
      <div className='flex flex-col gap-4'>
        {error ? (
          <Alert variant='destructive'>
            <AlertTriangle />
            <AlertTitle>{t('Failed to fetch credential status')}</AlertTitle>
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        ) : null}

        {isLoading ? (
          <div className='flex flex-col gap-3'>
            <Skeleton className='h-8 w-40' />
            <div className='grid grid-cols-1 gap-3 md:grid-cols-2'>
              <Skeleton className='h-16 w-full' />
              <Skeleton className='h-16 w-full' />
              <Skeleton className='h-16 w-full' />
              <Skeleton className='h-16 w-full' />
            </div>
          </div>
        ) : status ? (
          <>
            <div className='flex flex-wrap items-center gap-2'>
              <StatusBadge
                label={status.valid === false ? t('Invalid') : t('Valid')}
                variant={validVariant}
                copyable={false}
              />
              {status.expired ? (
                <StatusBadge
                  label={t('Expired')}
                  variant='warning'
                  copyable={false}
                />
              ) : null}
              {status.auto_refresh ? (
                <StatusBadge
                  label={t('Auto refresh')}
                  variant='info'
                  copyable={false}
                />
              ) : null}
              {status.provider ? (
                <StatusBadge
                  label={String(status.provider)}
                  variant='neutral'
                  copyable={false}
                />
              ) : null}
            </div>

            <div className='grid grid-cols-1 gap-3 md:grid-cols-2'>
              <InfoField label={t('Email')} value={status.email} copyable />
              <InfoField
                label={t('Account ID')}
                value={status.account_id}
                mono
                copyable
              />
              <InfoField
                label={t('Last refresh')}
                value={formatCredentialTime(status.last_refresh)}
                mono
                copyable={false}
              />
              <InfoField
                label={t('Remaining time')}
                value={formatRemainingSeconds(status.remaining_seconds, t)}
                mono
                copyable={false}
              />
              {channelLabel ? (
                <InfoField
                  label={t('Channel')}
                  value={channelLabel}
                  copyable={false}
                  className='md:col-span-2'
                />
              ) : null}
            </div>
          </>
        ) : null}
      </div>
    </Dialog>
  )
}
