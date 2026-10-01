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
import { Check, CreditCard, Gift, Loader2, QrCode, Sparkles, Tag, Wallet, Zap } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { api } from '@/lib/api'
import { formatQuotaWithCurrency } from '@/lib/currency'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

interface RechargeTier {
  amount: number
  bonus: number
  badge?: string
  popular?: boolean
}

const DEFAULT_TIERS: RechargeTier[] = [
  { amount: 10, bonus: 0 },
  { amount: 30, bonus: 3, badge: '赠 ¥3' },
  { amount: 50, bonus: 8, badge: '赠 ¥8', popular: true },
  { amount: 100, bonus: 20, badge: '赠 ¥20' },
  { amount: 200, bonus: 50, badge: '赠 ¥50' },
  { amount: 500, bonus: 150, badge: '赠 ¥150' },
]

type PayMethod = 'alipay' | 'wxpay' | 'card'

interface RechargeDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function RechargeDialog({ open, onOpenChange }: RechargeDialogProps) {
  const { t } = useTranslation()
  const user = useAuthStore((state) => state.auth.user)
  const refreshUser = useAuthStore((state) => state.auth.refreshUser)

  const [selectedTier, setSelectedTier] = useState<number>(50)
  const [customAmount, setCustomAmount] = useState<string>('')
  const [isCustom, setIsCustom] = useState<boolean>(false)
  const [payMethod, setPayMethod] = useState<PayMethod>('alipay')
  const [cardKey, setCardKey] = useState<string>('')
  const [loading, setLoading] = useState<boolean>(false)
  const [payUrl, setPayUrl] = useState<string | null>(null)

  const currentAmount = isCustom ? Number(customAmount) || 0 : selectedTier
  const currentTier = DEFAULT_TIERS.find((t) => t.amount === currentAmount)
  const currentBonus = isCustom ? 0 : currentTier?.bonus || 0
  const totalReceived = currentAmount + currentBonus

  const handleCardRedeem = async () => {
    if (!cardKey.trim()) {
      toast.error(t('Please enter card redemption key'))
      return
    }
    setLoading(true)
    try {
      const res = await api.post<{ success: boolean; message?: string }>('/api/user/topup', {
        key: cardKey.trim(),
      })
      if (res.data?.success) {
        toast.success(t('Card redeemed successfully!'))
        setCardKey('')
        if (refreshUser) await refreshUser()
        onOpenChange(false)
      } else {
        toast.error(res.data?.message || t('Redemption failed'))
      }
    } catch (e: any) {
      toast.error(e?.response?.data?.message || t('Network error during redemption'))
    } finally {
      setLoading(false)
    }
  }

  const handleOnlinePay = async () => {
    if (currentAmount <= 0) {
      toast.error(t('Please select a valid recharge amount'))
      return
    }
    setLoading(true)
    setPayUrl(null)
    try {
      const paymentType = payMethod === 'alipay' ? 'alipay' : 'wxpay'
      const res = await api.post<{
        success: boolean
        message?: string
        data?: { url?: string; qr_code?: string }
      }>('/api/user/pay', {
        amount: currentAmount,
        payment_method: paymentType,
      })
      if (res.data?.success && res.data.data?.url) {
        window.open(res.data.data.url, '_blank')
        toast.success(t('Redirecting to payment gateway...'))
        onOpenChange(false)
      } else {
        toast.error(res.data?.message || t('Failed to create payment order'))
      }
    } catch (e: any) {
      toast.error(e?.response?.data?.message || t('Payment initiation failed'))
    } finally {
      setLoading(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='max-w-[480px] border-white/10 bg-[#0f1117] text-white p-6 rounded-2xl shadow-2xl backdrop-blur-xl'>
        <DialogHeader className='text-left'>
          <div className='flex items-center justify-between'>
            <DialogTitle className='flex items-center gap-2 text-lg font-bold text-white'>
              <Wallet className='size-5 text-amber-400' />
              {t('Account Recharge & Credits')}
            </DialogTitle>
          </div>
          <DialogDescription className='text-xs text-gray-400 mt-1'>
            {t('Instant credit deposit, multi-model shared balance with official rates.')}
          </DialogDescription>
        </DialogHeader>

        {/* Current Balance */}
        <div className='flex items-center justify-between rounded-xl border border-white/[0.08] bg-white/[0.02] p-3.5 mt-2'>
          <div>
            <span className='text-xs text-gray-400 block'>{t('Current Available Balance')}</span>
            <span className='text-xl font-bold text-white flex items-center gap-1.5 mt-0.5'>
              <Zap className='size-4 text-amber-300' />
              {formatQuotaWithCurrency(user?.quota ?? 0)}
            </span>
          </div>
          <span className='text-[11px] font-medium text-amber-300 bg-amber-500/10 border border-amber-500/20 px-2.5 py-1 rounded-full flex items-center gap-1'>
            <Sparkles className='size-3' />
            {t('Official 1:1 Rate')}
          </span>
        </div>

        {/* Payment Method Tabs */}
        <div className='grid grid-cols-3 gap-2 mt-4'>
          <button
            type='button'
            onClick={() => setPayMethod('alipay')}
            className={cn(
              'flex items-center justify-center gap-2 rounded-lg border py-2.5 text-xs font-semibold transition-all',
              payMethod === 'alipay'
                ? 'border-blue-500 bg-blue-500/15 text-blue-300 shadow-[0_0_12px_rgba(59,130,246,0.25)]'
                : 'border-white/10 bg-white/[0.02] text-gray-400 hover:border-white/20 hover:text-white'
            )}
          >
            <CreditCard className='size-4' />
            {t('Alipay')}
          </button>
          <button
            type='button'
            onClick={() => setPayMethod('wxpay')}
            className={cn(
              'flex items-center justify-center gap-2 rounded-lg border py-2.5 text-xs font-semibold transition-all',
              payMethod === 'wxpay'
                ? 'border-emerald-500 bg-emerald-500/15 text-emerald-300 shadow-[0_0_12px_rgba(16,185,129,0.25)]'
                : 'border-white/10 bg-white/[0.02] text-gray-400 hover:border-white/20 hover:text-white'
            )}
          >
            <QrCode className='size-4' />
            {t('WeChat Pay')}
          </button>
          <button
            type='button'
            onClick={() => setPayMethod('card')}
            className={cn(
              'flex items-center justify-center gap-2 rounded-lg border py-2.5 text-xs font-semibold transition-all',
              payMethod === 'card'
                ? 'border-amber-500 bg-amber-500/15 text-amber-300 shadow-[0_0_12px_rgba(245,158,11,0.25)]'
                : 'border-white/10 bg-white/[0.02] text-gray-400 hover:border-white/20 hover:text-white'
            )}
          >
            <Tag className='size-4' />
            {t('Card Key')}
          </button>
        </div>

        {/* Online Pay: Tiers selection */}
        {payMethod !== 'card' ? (
          <div className='mt-4 space-y-3'>
            <div className='grid grid-cols-3 gap-2.5'>
              {DEFAULT_TIERS.map((tier) => {
                const active = !isCustom && selectedTier === tier.amount
                return (
                  <button
                    key={tier.amount}
                    type='button'
                    onClick={() => {
                      setIsCustom(false)
                      setSelectedTier(tier.amount)
                    }}
                    className={cn(
                      'relative flex flex-col items-center justify-center rounded-xl border p-3.5 transition-all',
                      active
                        ? 'border-cyan-400 bg-cyan-500/10 shadow-[0_0_15px_rgba(34,211,238,0.2)] text-white'
                        : 'border-white/[0.08] bg-white/[0.02] text-gray-300 hover:border-white/20 hover:bg-white/[0.04]'
                    )}
                  >
                    {tier.badge && (
                      <span className='absolute -top-2.5 right-2 rounded-full bg-gradient-to-r from-pink-500 to-rose-500 px-1.5 py-0.5 text-[9px] font-bold text-white shadow-sm'>
                        {tier.badge}
                      </span>
                    )}
                    <span className='text-lg font-black'>¥{tier.amount}</span>
                    <span className='text-[10px] text-gray-400 mt-0.5'>
                      {tier.bonus > 0 ? `得 ¥${tier.amount + tier.bonus}` : '基础额度'}
                    </span>
                  </button>
                )
              })}
            </div>

            {/* Custom Amount input */}
            <div className='relative mt-2'>
              <input
                type='number'
                min='1'
                step='1'
                value={customAmount}
                onChange={(e) => {
                  setIsCustom(true)
                  setCustomAmount(e.target.value)
                }}
                onFocus={() => setIsCustom(true)}
                placeholder={t('Or enter custom amount (¥)...')}
                className={cn(
                  'h-10 w-full rounded-xl border bg-white/[0.03] px-3.5 text-xs text-white placeholder:text-gray-500 focus:outline-none transition-all',
                  isCustom
                    ? 'border-cyan-400 bg-cyan-500/5 ring-1 ring-cyan-400/30'
                    : 'border-white/10 hover:border-white/20'
                )}
              />
            </div>

            {/* Summary */}
            <div className='flex items-center justify-between rounded-lg bg-white/[0.02] px-3.5 py-2.5 text-xs text-gray-400'>
              <span>{t('Total Credits to receive')}:</span>
              <span className='text-sm font-bold text-amber-300'>
                ¥{totalReceived.toFixed(2)}
                {currentBonus > 0 && (
                  <span className='ml-1 text-[11px] font-normal text-rose-400'>
                    (含加赠 ¥{currentBonus})
                  </span>
                )}
              </span>
            </div>

            {/* Pay Button */}
            <button
              type='button'
              disabled={loading || currentAmount <= 0}
              onClick={handleOnlinePay}
              className='mt-2 flex h-11 w-full items-center justify-center gap-2 rounded-xl bg-gradient-to-r from-cyan-500 to-blue-600 font-bold text-white text-sm shadow-[0_0_20px_rgba(34,211,238,0.3)] transition-all hover:opacity-95 disabled:opacity-50'
            >
              {loading ? (
                <Loader2 className='size-4 animate-spin' />
              ) : (
                <>
                  <Zap className='size-4' />
                  {t(`Pay ¥${currentAmount} via ${payMethod === 'alipay' ? 'Alipay' : 'WeChat'}`)}
                </>
              )}
            </button>
          </div>
        ) : (
          /* Card Redeem Section */
          <div className='mt-4 space-y-3'>
            <div className='space-y-1.5'>
              <label className='text-xs text-gray-300 font-medium block'>
                {t('Card Key / Redemption Code')}
              </label>
              <input
                type='text'
                value={cardKey}
                onChange={(e) => setCardKey(e.target.value)}
                placeholder={t('Enter 16-32 digit redemption code...')}
                className='h-11 w-full rounded-xl border border-white/10 bg-white/[0.03] px-3.5 text-xs text-white placeholder:text-gray-500 focus:border-amber-400 focus:ring-1 focus:ring-amber-400/30 focus:outline-none'
              />
            </div>

            <div className='rounded-xl border border-amber-500/20 bg-amber-500/[0.05] p-3 text-xs text-amber-300/90 leading-5'>
              <div className='flex items-center gap-1.5 font-semibold text-amber-300 mb-1'>
                <Gift className='size-3.5' />
                {t('Automatic & Instant Deposit')}
              </div>
              {t('Card keys can be redeemed directly without gateway redirection.')}
            </div>

            <button
              type='button'
              disabled={loading || !cardKey.trim()}
              onClick={handleCardRedeem}
              className='mt-2 flex h-11 w-full items-center justify-center gap-2 rounded-xl bg-gradient-to-r from-amber-400 to-yellow-500 font-bold text-amber-950 text-sm shadow-[0_0_20px_rgba(245,158,11,0.3)] transition-all hover:opacity-95 disabled:opacity-50'
            >
              {loading ? (
                <Loader2 className='size-4 animate-spin' />
              ) : (
                <>
                  <Check className='size-4' />
                  {t('Redeem Now')}
                </>
              )}
            </button>
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}
