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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Spinner } from '@/components/ui/spinner'
import { Textarea } from '@/components/ui/textarea'
import { handleServerError } from '@/lib/handle-server-error'
import { getServerErrorMessage } from '@/lib/server-error-message'

import { importAccountPool } from '../api'
import type { PoolImportResultItem } from '../types'

type PoolImportDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  onImported: () => void
}

export function PoolImportDialog(props: PoolImportDialogProps) {
  const { t } = useTranslation()
  const [raw, setRaw] = useState('')
  const [group, setGroup] = useState('')
  const [models, setModels] = useState('')
  const [maxConcurrency, setMaxConcurrency] = useState('')
  const [expiresDays, setExpiresDays] = useState('')
  const [busy, setBusy] = useState<'preview' | 'import' | null>(null)
  const [preview, setPreview] = useState<PoolImportResultItem[] | null>(null)

  const reset = () => {
    setRaw('')
    setGroup('')
    setModels('')
    setMaxConcurrency('')
    setExpiresDays('')
    setPreview(null)
  }

  const handleOpenChange = (open: boolean) => {
    props.onOpenChange(open)
    if (!open) {
      reset()
    }
  }

  const run = async (dryRun: boolean) => {
    if (!raw.trim()) {
      toast.error(t('Please paste the account data first'))
      return
    }
    setBusy(dryRun ? 'preview' : 'import')
    try {
      const res = await importAccountPool({
        raw,
        group: group.trim(),
        models: models.trim(),
        dry_run: dryRun,
        max_concurrency: Number(maxConcurrency) || 0,
        expires_days: Number(expiresDays) || 0,
      })
      if (!res.success) {
        toast.error(getServerErrorMessage(res, t('Failed to import accounts')))
        return
      }
      if (dryRun) {
        setPreview(res.data.results)
        return
      }
      if (res.data.created > 0) {
        toast.success(t('Imported {{count}} accounts', { count: res.data.created }))
        props.onImported()
        handleOpenChange(false)
      } else {
        setPreview(res.data.results)
        toast.error(t('No accounts were imported'))
      }
    } catch (error) {
      handleServerError(error, t('Failed to import accounts'))
    } finally {
      setBusy(null)
    }
  }

  return (
    <Dialog open={props.open} onOpenChange={handleOpenChange}>
      <DialogContent className='sm:max-w-2xl'>
        <DialogHeader>
          <DialogTitle>{t('Import accounts')}</DialogTitle>
          <DialogDescription>
            {t(
              'Paste credential data: Claude rt-JSON, ChatGPT auth.json, Google exports or this system\u2019s native credential JSON. JSON arrays, JSON lines and concatenated objects are all supported; the provider is auto-detected.'
            )}
            <br />
            {t(
              'API key accounts are also supported: paste one key per line, or JSON entries like {"api_key":"...","base_url":"https://..."}.'
            )}
          </DialogDescription>
        </DialogHeader>
        <div className='grid gap-3'>
          <Textarea
            value={raw}
            onChange={(e) => {
              setRaw(e.target.value)
              setPreview(null)
            }}
            placeholder={'sk-...\n{"api_key":"sk-...","base_url":"https://api.example.com"}'}
            rows={8}
            className='font-mono text-xs'
          />
          <div className='flex items-center gap-2'>
            <Label className='shrink-0' htmlFor='pool-import-group'>
              {t('Group')}
            </Label>
            <Input
              id='pool-import-group'
              value={group}
              onChange={(e) => setGroup(e.target.value)}
              placeholder='default'
            />
          </div>
          <div className='grid gap-1.5'>
            <Label className='shrink-0' htmlFor='pool-import-models'>
              {t('Models')}
            </Label>
            <Input
              id='pool-import-models'
              value={models}
              onChange={(e) => setModels(e.target.value)}
              placeholder='gpt-4o,gpt-4o-mini,claude-sonnet-4-5,gemini-2.5-flash'
              className='font-mono text-xs'
            />
            <p className='text-muted-foreground text-xs'>
              {t('Leave empty to use the provider default model list.')}
            </p>
          </div>
          <div className='flex items-center gap-2'>
            <Label className='shrink-0' htmlFor='pool-import-concurrency'>
              {t('Max concurrency')}
            </Label>
            <Input
              id='pool-import-concurrency'
              type='number'
              min={0}
              step={1}
              className='w-24'
              value={maxConcurrency}
              onChange={(e) => setMaxConcurrency(e.target.value)}
              placeholder='0'
            />
            <Label className='shrink-0' htmlFor='pool-import-expires'>
              {t('Expires in (days)')}
            </Label>
            <Input
              id='pool-import-expires'
              type='number'
              min={0}
              step={1}
              className='w-24'
              value={expiresDays}
              onChange={(e) => setExpiresDays(e.target.value)}
              placeholder='0'
            />
          </div>
          {preview ? (
            <div className='max-h-56 space-y-1 overflow-auto rounded-md border p-2 text-xs'>
              {preview.map((item) => (
                <div key={item.index} className='flex items-center gap-2'>
                  <Badge variant='outline' className='shrink-0 tabular-nums'>
                    #{item.index}
                  </Badge>
                  <span className='shrink-0'>{item.provider || '-'}</span>
                  <span className='truncate'>
                    {item.email || item.base_url || item.name || ''}
                  </span>
                  {item.base_url ? (
                    <span className='text-muted-foreground shrink-0 truncate'>
                      {item.base_url}
                    </span>
                  ) : null}
                  {item.error ? (
                    <span className='text-destructive ml-auto truncate'>
                      {item.error}
                    </span>
                  ) : item.name ? (
                    <span className='text-muted-foreground ml-auto truncate'>
                      {item.name}
                    </span>
                  ) : null}
                </div>
              ))}
            </div>
          ) : null}
        </div>
        <DialogFooter>
          <Button
            variant='outline'
            disabled={busy !== null}
            onClick={() => void run(true)}
          >
            {busy === 'preview' ? <Spinner /> : null}
            {t('Preview')}
          </Button>
          <Button disabled={busy !== null} onClick={() => void run(false)}>
            {busy === 'import' ? <Spinner /> : null}
            {t('Import')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
