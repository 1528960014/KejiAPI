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
import {
  AlertTriangle,
  Bot,
  Check,
  Copy,
  ExternalLink,
  Feather,
  Loader2,
  Plus,
} from 'lucide-react'
import { type ReactNode, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import {
  createServerError,
  getServerErrorMessage,
} from '@/lib/server-error-message'
import { cn } from '@/lib/utils'

import {
  createSubscriptionChannel,
  getChannelSubscriptionAuthUrl,
  type SubscriptionProvider,
} from '../../api'
import { channelsQueryKeys } from '../../lib'

type SubscriptionAccountDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

type ProviderCardProps = {
  icon: ReactNode
  title: string
  description: string
  selected: boolean
  onSelect: () => void
}

function ProviderCard(props: ProviderCardProps) {
  return (
    <button
      type='button'
      onClick={props.onSelect}
      aria-pressed={props.selected}
      className={cn(
        'flex flex-col items-start gap-2 rounded-xl border p-4 text-left transition-colors hover:bg-muted/40',
        props.selected && 'border-ring bg-muted/40'
      )}
    >
      <div className='flex w-full items-center gap-2'>
        <span className='text-muted-foreground [&_svg]:size-4'>
          {props.icon}
        </span>
        <span className='text-sm font-semibold'>{props.title}</span>
        {props.selected ? (
          <Check className='text-success ml-auto size-4 shrink-0' />
        ) : null}
      </div>
      <span className='text-muted-foreground text-xs leading-5'>
        {props.description}
      </span>
    </button>
  )
}

export function SubscriptionAccountDialog({
  open,
  onOpenChange,
}: SubscriptionAccountDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { copiedText, copyToClipboard } = useCopyToClipboard({ notify: false })
  const [step, setStep] = useState<1 | 2 | 3>(1)
  const [provider, setProvider] = useState<SubscriptionProvider | null>(null)
  const [authUrl, setAuthUrl] = useState('')
  const [codeVerifier, setCodeVerifier] = useState('')
  const [isFetchingLink, setIsFetchingLink] = useState(false)
  const [code, setCode] = useState('')
  const [name, setName] = useState('')
  const [group, setGroup] = useState('')
  const [isCreating, setIsCreating] = useState(false)
  const [error, setError] = useState('')
  const [createdName, setCreatedName] = useState('')

  const resetState = () => {
    setStep(1)
    setProvider(null)
    setAuthUrl('')
    setCodeVerifier('')
    setIsFetchingLink(false)
    setCode('')
    setName('')
    setGroup('')
    setIsCreating(false)
    setError('')
    setCreatedName('')
  }

  const handleOpenChange = (nextOpen: boolean) => {
    if (!nextOpen) {
      resetState()
    }
    onOpenChange(nextOpen)
  }

  const handleSelectProvider = (value: SubscriptionProvider) => {
    setProvider(value)
    setError('')
    setStep(2)
  }

  const handleGetLoginLink = async () => {
    if (!provider || isFetchingLink) {
      return
    }
    setIsFetchingLink(true)
    setError('')
    setAuthUrl('')
    setCodeVerifier('')
    try {
      const res = await getChannelSubscriptionAuthUrl(provider)
      if (!res.success) {
        throw createServerError(res, t('Failed to get login link'))
      }
      const url = res.data?.auth_url?.trim() ?? ''
      if (!url) {
        throw createServerError(res, t('Failed to get login link'))
      }
      setAuthUrl(url)
      setCodeVerifier(res.data?.code_verifier ?? '')
    } catch (err) {
      setError(getServerErrorMessage(err, t('Failed to get login link')))
    } finally {
      setIsFetchingLink(false)
    }
  }

  const handleCreate = async () => {
    if (!provider || !code.trim() || isCreating) {
      return
    }
    setIsCreating(true)
    setError('')
    try {
      const res = await createSubscriptionChannel({
        provider,
        code: code.trim(),
        ...(provider === 'gpt' && codeVerifier ? { code_verifier: codeVerifier } : {}),
        ...(name.trim() ? { name: name.trim() } : {}),
        ...(group.trim() ? { group: group.trim() } : {}),
      })
      if (!res.success) {
        throw createServerError(res, t('Failed to create channel'))
      }
      const created = res.data?.name?.trim() || `#${res.data?.id ?? ''}`
      toast.success(t('Channel created: {{name}}', { name: created }))
      await queryClient.invalidateQueries({
        queryKey: channelsQueryKeys.lists(),
      })
      setCreatedName(created)
    } catch (err) {
      setError(getServerErrorMessage(err, t('Failed to create channel')))
    } finally {
      setIsCreating(false)
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={handleOpenChange}
      title={t('Add Subscription Account')}
      description={t(
        'Sign in with a subscription account and create a channel automatically.'
      )}
      contentHeight='auto'
      bodyClassName='flex flex-col gap-4'
      footer={
        <Button
          type='button'
          variant='outline'
          onClick={() => handleOpenChange(false)}
        >
          {t('Close')}
        </Button>
      }
    >
      <div className='flex flex-col gap-4'>
        {error ? (
          <Alert variant='destructive'>
            <AlertTriangle />
            <AlertTitle>{t('Something went wrong!')}</AlertTitle>
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        ) : null}

        {step === 1 ? (
          <div className='flex flex-col gap-3'>
            <div className='text-sm font-medium'>
              {t('Select Subscription Provider')}
            </div>
            <div className='grid grid-cols-1 gap-3 sm:grid-cols-2'>
              <ProviderCard
                icon={<Feather />}
                title={t('Claude Subscription')}
                description={t(
                  'Sign in with your Claude subscription account to create a channel.'
                )}
                selected={provider === 'claude'}
                onSelect={() => handleSelectProvider('claude')}
              />
              <ProviderCard
                icon={<Bot />}
                title={t('ChatGPT Subscription')}
                description={t(
                  'Sign in with your ChatGPT subscription account to create a channel.'
                )}
                selected={provider === 'gpt'}
                onSelect={() => handleSelectProvider('gpt')}
              />
            </div>
          </div>
        ) : null}

        {step === 2 ? (
          <div className='flex flex-col gap-3'>
            <div className='text-sm font-medium'>{t('Login Link')}</div>
            <div className='text-muted-foreground text-xs leading-5'>
              {t(
                'Open the link in a new window and complete the login with your subscription account.'
              )}
            </div>
            <div className='flex items-center gap-2'>
              <Input
                id='subscription-auth-url'
                readOnly
                value={authUrl}
                placeholder={t('Click "Get Login Link" to generate')}
                className='font-mono text-xs'
              />
              <Button
                type='button'
                variant='outline'
                size='sm'
                className='shrink-0'
                onClick={() => void copyToClipboard(authUrl)}
                disabled={!authUrl}
              >
                {authUrl && copiedText === authUrl ? (
                  <Check data-icon='inline-start' className='text-success' />
                ) : (
                  <Copy data-icon='inline-start' />
                )}
                {t('Copy')}
              </Button>
              <Button
                type='button'
                variant='outline'
                size='sm'
                className='shrink-0'
                onClick={() => window.open(authUrl, '_blank')}
                disabled={!authUrl}
              >
                <ExternalLink data-icon='inline-start' />
                {t('Open')}
              </Button>
            </div>
            <div className='flex items-center justify-between gap-2 pt-1'>
              <Button
                type='button'
                variant='ghost'
                size='sm'
                onClick={() => setStep(1)}
              >
                {t('Back')}
              </Button>
              <div className='flex items-center gap-2'>
                <Button
                  type='button'
                  variant='outline'
                  onClick={() => void handleGetLoginLink()}
                  disabled={isFetchingLink || !provider}
                >
                  {isFetchingLink ? (
                    <Loader2 data-icon='inline-start' className='animate-spin' />
                  ) : (
                    <ExternalLink data-icon='inline-start' />
                  )}
                  {isFetchingLink
                    ? t('Fetching login link...')
                    : t('Get Login Link')}
                </Button>
                <Button type='button' onClick={() => setStep(3)} disabled={!authUrl}>
                  {t('Next')}
                </Button>
              </div>
            </div>
          </div>
        ) : null}

        {step === 3 ? (
          <div className='flex flex-col gap-3'>
            <div className='flex items-center justify-between gap-2'>
              <div className='text-sm font-medium'>
                {t('Complete Channel Creation')}
              </div>
              <Button
                type='button'
                variant='ghost'
                size='sm'
                onClick={() => setStep(2)}
              >
                {t('Back')}
              </Button>
            </div>

            <div className='flex flex-col gap-1.5'>
              <Label htmlFor='subscription-auth-code'>
                {t('Authorization Code')}
              </Label>
              <Textarea
                id='subscription-auth-code'
                value={code}
                onChange={(e) => setCode(e.target.value)}
                placeholder={t(
                  'Paste the authorization code shown on the page after signing in'
                )}
                rows={4}
              />
            </div>

            <div className='grid grid-cols-1 gap-3 sm:grid-cols-2'>
              <div className='flex flex-col gap-1.5'>
                <Label htmlFor='subscription-channel-name'>{t('Name')}</Label>
                <Input
                  id='subscription-channel-name'
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder={t('Optional')}
                />
              </div>
              <div className='flex flex-col gap-1.5'>
                <Label htmlFor='subscription-channel-group'>{t('Group')}</Label>
                <Input
                  id='subscription-channel-group'
                  value={group}
                  onChange={(e) => setGroup(e.target.value)}
                  placeholder={t('Optional')}
                />
              </div>
            </div>

            {createdName ? (
              <Alert className='border-success/40 bg-success/10 text-success'>
                <Check />
                <AlertTitle>{t('Channel created')}</AlertTitle>
                <AlertDescription>{createdName}</AlertDescription>
              </Alert>
            ) : null}

            <Button
              type='button'
              onClick={() => void handleCreate()}
              disabled={isCreating || !code.trim() || !provider}
            >
              {isCreating ? (
                <Loader2 data-icon='inline-start' className='animate-spin' />
              ) : (
                <Plus data-icon='inline-start' />
              )}
              {isCreating ? t('Creating...') : t('Create Channel')}
            </Button>
          </div>
        ) : null}
      </div>
    </Dialog>
  )
}
