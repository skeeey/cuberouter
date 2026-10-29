/*
Copyright (C) 2023-2026 QuantumNous

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

For commercial licensing, please contact support@quantumnous.com
*/
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import {
  formatQuotaToCents,
  getCurrencyDisplay,
  getCurrencyLabel,
} from '@/lib/currency'
import { parseQuotaFromDollars } from '@/lib/format'
import { cn } from '@/lib/utils'

import { adjustUserQuota } from '../api'
import type { QuotaAccountType, User } from '../types'

interface UserQuotaDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  user: User
  currentQuota: number
  onSuccess: () => void
}

export function UserQuotaDialog(props: UserQuotaDialogProps) {
  const { t } = useTranslation()
  const [accountType, setAccountType] = useState<QuotaAccountType>('gift')
  const [amount, setAmount] = useState('')
  const [remark, setRemark] = useState('')
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    if (!props.open) return
    setAmount('')
    setRemark('')
    setAccountType('gift')
  }, [props.open, props.user.id])

  const { meta: currencyMeta } = getCurrencyDisplay()
  const currencyLabel = getCurrencyLabel()
  const tokensOnly = currencyMeta.kind === 'tokens'
  const currencySymbol = tokensOnly ? '' : currencyMeta.symbol

  const amountValue = Number.parseFloat(amount) || 0
  const quotaValue =
    amountValue > 0 ? parseQuotaFromDollars(amountValue) : 0

  // 金额输入只允许两位小数（分）；tokens 模式按整数输入。
  const handleAmountChange = (value: string) => {
    if (tokensOnly || value === '' || /^\d*\.?\d{0,2}$/.test(value)) {
      setAmount(value)
    }
  }

  const cashTotal = props.user.cash_topup_total
  const giftTotal = props.user.gift_topup_total
  const breakdownLoaded =
    typeof cashTotal === 'number' || typeof giftTotal === 'number'

  const displayName =
    props.user.display_name || props.user.username || String(props.user.id)
  const avatarText = [...displayName][0]?.toUpperCase() ?? '?'

  const handleConfirm = async () => {
    if (quotaValue <= 0) return

    setLoading(true)
    try {
      const result = await adjustUserQuota({
        id: props.user.id,
        action: 'add_quota',
        mode: 'add',
        value: quotaValue,
        account_type: accountType,
        remark: remark.trim() || undefined,
      })
      if (result.success) {
        toast.success(t('Top-up completed'))
        props.onOpenChange(false)
        props.onSuccess()
      } else {
        toast.error(result.message || t('Failed to adjust quota'))
      }
    } catch (e: unknown) {
      toast.error(e instanceof Error ? e.message : t('Failed to adjust quota'))
    } finally {
      setLoading(false)
    }
  }

  const placeholder = tokensOnly
    ? t('Enter amount in tokens')
    : t('Enter amount in {{currency}}', { currency: currencyLabel })

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('Top-up')}
      contentClassName='sm:max-w-md'
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <>
          <Button variant='outline' onClick={() => props.onOpenChange(false)}>
            {t('Cancel')}
          </Button>
          <Button onClick={handleConfirm} disabled={loading || quotaValue <= 0}>
            {loading ? t('Processing...') : t('Confirm')}
          </Button>
        </>
      }
    >
      <div className='space-y-4'>
        <div className='bg-muted/30 flex items-center gap-3 rounded-lg border p-3'>
          <div className='bg-primary/10 text-primary flex size-10 shrink-0 items-center justify-center rounded-full text-sm font-semibold'>
            {avatarText}
          </div>
          <div className='min-w-0'>
            <div className='text-foreground truncate text-sm font-medium'>
              {displayName}
            </div>
            {props.user.email ? (
              <div className='text-muted-foreground truncate text-xs'>
                {props.user.email}
              </div>
            ) : null}
            <div className='text-muted-foreground mt-1 text-xs'>
              {t('Current quota')}: {formatQuotaToCents(props.currentQuota)}
            </div>
            {breakdownLoaded ? (
              <div className='text-muted-foreground text-xs'>
                {t('Cash')} {formatQuotaToCents(cashTotal ?? 0)} / {t('Gift')}{' '}
                {formatQuotaToCents(giftTotal ?? 0)}
              </div>
            ) : null}
          </div>
        </div>

        <div className='space-y-2'>
          <Label>{t('Credit Account')}</Label>
          <div className='grid grid-cols-2 gap-2'>
            {(['gift', 'cash'] as const).map((type) => (
              <Button
                key={type}
                type='button'
                variant='outline'
                aria-pressed={accountType === type}
                className={cn(
                  accountType === type &&
                    'bg-primary text-primary-foreground hover:bg-primary/90 hover:text-primary-foreground'
                )}
                onClick={() => setAccountType(type)}
              >
                {type === 'gift' ? t('Gift Account') : t('Cash Account')}
              </Button>
            ))}
          </div>
        </div>

        <div className='space-y-2'>
          <Label>
            {t('Recharge Amount')} ({currencyLabel})
          </Label>
          <div className='relative'>
            {currencySymbol ? (
              <span className='text-muted-foreground pointer-events-none absolute inset-y-0 left-3 flex items-center text-sm'>
                {currencySymbol}
              </span>
            ) : null}
            <Input
              type='number'
              step={tokensOnly ? 1 : 0.01}
              min={0}
              className={cn(currencySymbol && 'pl-8')}
              placeholder={placeholder}
              value={amount}
              onChange={(e) => handleAmountChange(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') handleConfirm()
              }}
            />
          </div>
          <div className='text-muted-foreground text-xs'>
            {t('Balance after top-up')}:{' '}
            {formatQuotaToCents(props.currentQuota + quotaValue)}
          </div>
        </div>

        <div className='space-y-2'>
          <Label>{t('Remark')}</Label>
          <Textarea
            rows={3}
            maxLength={255}
            value={remark}
            onChange={(e) => setRemark(e.target.value)}
          />
        </div>
      </div>
    </Dialog>
  )
}
