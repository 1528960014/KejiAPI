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
import { Loader2, ShieldCheck } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

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
import { Separator } from '@/components/ui/separator'
import { Switch } from '@/components/ui/switch'
import { handleServerError } from '@/lib/handle-server-error'

import { useAccountPool, useUpdateAccountPoolSettings } from '../api'
import type { PoolSettings } from '../types'

type PoolSettingsDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function PoolSettingsDialog({ open, onOpenChange }: PoolSettingsDialogProps) {
  const { t } = useTranslation()
  const poolQuery = useAccountPool()
  const serverSettings = poolQuery.data?.data?.settings
  const updateSettings = useUpdateAccountPoolSettings()
  const [form, setForm] = useState<PoolSettings | null>(null)
  const [syncedSettings, setSyncedSettings] = useState<PoolSettings | null>(null)

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
      onSuccess: () => {
        toast.success(t('Settings saved successfully'))
        onOpenChange(false)
      },
      onError: (error) => handleServerError(error, t('Failed to save settings')),
    })
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='max-h-[85vh] overflow-y-auto sm:max-w-xl'>
        <DialogHeader>
          <DialogTitle className='flex items-center gap-2'>
            <ShieldCheck className='size-5 text-emerald-500' />
            {t('Account Pool Settings')}
          </DialogTitle>
          <DialogDescription>
            {t(
              'Scheduling, session stickiness, cooldowns, isolation and rate limiting for pooled accounts.'
            )}
          </DialogDescription>
        </DialogHeader>

        <div className='grid gap-4 py-2 text-sm'>
          {/* 冷却保护 */}
          <div className='flex items-center justify-between gap-3'>
            <div className='min-w-0'>
              <Label className='font-medium'>{t('Cooldown Protection')}</Label>
              <p className='text-muted-foreground mt-0.5 text-xs'>
                {t(
                  'Temporarily hold auto-disabled accounts out of rotation for a fixed duration.'
                )}
              </p>
            </div>
            <div className='flex items-center gap-2 shrink-0'>
              {form?.cooldown_enabled ? (
                <div className='flex items-center gap-1.5'>
                  <Input
                    type='number'
                    min={0}
                    step={1}
                    className='h-8 w-20'
                    value={form.cooldown_minutes}
                    onChange={(event) =>
                      updateField(
                        'cooldown_minutes',
                        Math.max(0, Number(event.target.value) || 0)
                      )
                    }
                  />
                  <span className='text-muted-foreground text-xs'>
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

          {/* 429 速率限制冷却 */}
          <div className='flex items-center justify-between gap-3'>
            <div className='min-w-0'>
              <Label className='font-medium'>{t('Rate Limit Cooldown')}</Label>
              <p className='text-muted-foreground mt-0.5 text-xs'>
                {t(
                  'Cooldown applied when the upstream answers 429. Rate-limit windows are short, so this stays in seconds.'
                )}
              </p>
            </div>
            <div className='flex items-center gap-2 shrink-0'>
              <Input
                type='number'
                min={5}
                step={1}
                className='h-8 w-20'
                value={form?.rate_limit_cooldown_seconds ?? 0}
                onChange={(event) =>
                  updateField(
                    'rate_limit_cooldown_seconds',
                    Math.max(5, Number(event.target.value) || 0)
                  )
                }
              />
              <span className='text-muted-foreground text-xs'>
                {t('seconds')}
              </span>
            </div>
          </div>

          <Separator />

          {/* 529 过载冷却 */}
          <div className='flex items-center justify-between gap-3'>
            <div className='min-w-0'>
              <Label className='font-medium'>{t('Overload Cooldown')}</Label>
              <p className='text-muted-foreground mt-0.5 text-xs'>
                {t(
                  'Isolation time applied when the upstream answers 529 (overloaded).'
                )}
              </p>
            </div>
            <div className='flex items-center gap-2 shrink-0'>
              <Input
                type='number'
                min={1}
                step={1}
                className='h-8 w-20'
                value={form?.overload_cooldown_minutes ?? 0}
                onChange={(event) =>
                  updateField(
                    'overload_cooldown_minutes',
                    Math.max(1, Number(event.target.value) || 0)
                  )
                }
              />
              <span className='text-muted-foreground text-xs'>
                {t('minutes')}
              </span>
            </div>
          </div>

          <Separator />

          {/* 凭据冷却 */}
          <div className='flex items-center justify-between gap-3'>
            <div className='min-w-0'>
              <Label className='font-medium'>{t('Credential Cooldown')}</Label>
              <p className='text-muted-foreground mt-0.5 text-xs'>
                {t(
                  'Recovery time for temporary credential errors (401 / 403 without an explicit ban).'
                )}
              </p>
            </div>
            <div className='flex items-center gap-2 shrink-0'>
              <Input
                type='number'
                min={1}
                step={1}
                className='h-8 w-20'
                value={form?.credential_cooldown_minutes ?? 0}
                onChange={(event) =>
                  updateField(
                    'credential_cooldown_minutes',
                    Math.max(1, Number(event.target.value) || 0)
                  )
                }
              />
              <span className='text-muted-foreground text-xs'>
                {t('minutes')}
              </span>
            </div>
          </div>

          <Separator />

          {/* 选择深度 */}
          <div className='flex items-center justify-between gap-3'>
            <div className='min-w-0'>
              <Label className='font-medium'>{t('Selection Depth')}</Label>
              <p className='text-muted-foreground mt-0.5 text-xs'>
                {t(
                  'How many of the healthiest pool accounts take part in each weighted draw. Lower concentrates traffic on healthy accounts; 0 disables load-aware ordering.'
                )}
              </p>
            </div>
            <div className='flex items-center gap-2 shrink-0'>
              <Input
                type='number'
                min={0}
                step={1}
                className='h-8 w-20'
                value={form?.selection_depth ?? 0}
                onChange={(event) =>
                  updateField(
                    'selection_depth',
                    Math.max(0, Number(event.target.value) || 0)
                  )
                }
              />
            </div>
          </div>

          <Separator />

          {/* 速率限制 */}
          <div className='flex items-center justify-between gap-3'>
            <div className='min-w-0'>
              <Label className='font-medium'>{t('Rate Limiting')}</Label>
              <p className='text-muted-foreground mt-0.5 text-xs'>
                {t(
                  'Cap incoming requests per account over a rolling minute window.'
                )}
              </p>
            </div>
            <div className='flex items-center gap-2 shrink-0'>
              {form?.rate_limit_enabled ? (
                <>
                  <div className='flex items-center gap-1.5'>
                    <Input
                      type='number'
                      min={1}
                      step={1}
                      className='h-8 w-20'
                      value={form.rate_limit_requests}
                      onChange={(event) =>
                        updateField(
                          'rate_limit_requests',
                          Math.max(1, Number(event.target.value) || 1)
                        )
                      }
                    />
                    <span className='text-muted-foreground text-xs'>
                      {t('requests')}
                    </span>
                  </div>
                  <div className='flex items-center gap-1.5'>
                    <Input
                      type='number'
                      min={1}
                      step={1}
                      className='h-8 w-16'
                      value={form.rate_limit_window_minutes}
                      onChange={(event) =>
                        updateField(
                          'rate_limit_window_minutes',
                          Math.max(1, Number(event.target.value) || 1)
                        )
                      }
                    />
                    <span className='text-muted-foreground text-xs'>
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

          {/* 会话粘性 */}
          <div className='flex items-center justify-between gap-3'>
            <div className='min-w-0'>
              <Label className='font-medium'>{t('Session Stickiness')}</Label>
              <p className='text-muted-foreground mt-0.5 text-xs'>
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
        </div>

        <DialogFooter className='gap-2 sm:gap-0'>
          <Button variant='outline' onClick={() => onOpenChange(false)}>
            {t('Cancel')}
          </Button>
          <Button
            onClick={handleSave}
            disabled={!form || !isDirty || updateSettings.isPending}
          >
            {updateSettings.isPending ? (
              <Loader2 className='size-4 animate-spin' />
            ) : null}
            {t('Save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
