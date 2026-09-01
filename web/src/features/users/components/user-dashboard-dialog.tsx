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
import { VChart } from '@visactor/react-vchart'
import { ChartColumnBig, Loader2 } from 'lucide-react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { getDashboardChartColors } from '@/features/dashboard/lib/charts'
import type { QuotaDataItem } from '@/features/dashboard/types'
import { formatQuota } from '@/lib/format'
import { formatChartTime } from '@/lib/time'
import { useChartTheme } from '@/lib/use-chart-theme'
import { VCHART_OPTION } from '@/lib/vchart'

import { getUserQuotaDates } from '../api'
import type { UserDashboardBrief } from '../types'

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  userId: number
  username: string
}

const RANGE_PRESETS = [
  { days: 7, labelKey: '7 days' },
  { days: 14, labelKey: '14 days' },
  { days: 30, labelKey: '30 days' },
] as const

const DAY_SECONDS = 24 * 60 * 60

export function UserDashboardDialog({
  open,
  onOpenChange,
  userId,
  username,
}: Props) {
  const { t } = useTranslation()
  const { resolvedTheme, themeReady } = useChartTheme()
  const [brief, setBrief] = useState<UserDashboardBrief | null>(null)
  const [dates, setDates] = useState<QuotaDataItem[]>([])
  const [rangeDays, setRangeDays] = useState<number>(7)
  const [loading, setLoading] = useState(false)
  // Stale-response guard: only the latest fetch may write state, so a slow
  // older range cannot overwrite a newer selection when it completes.
  const requestIdRef = useRef(0)

  const fetchDashboard = useCallback(
    async (days: number) => {
      const requestId = ++requestIdRef.current
      setLoading(true)
      try {
        const end = Math.floor(Date.now() / 1000)
        const start = end - days * DAY_SECONDS
        const res = await getUserQuotaDates(userId, {
          start_timestamp: start,
          end_timestamp: end,
        })
        if (requestId !== requestIdRef.current) return
        if (res.success && res.data) {
          setBrief(res.data.user)
          setDates(res.data.dates)
        } else {
          // e.g. 无权查看该用户的数据看板 — backend message is localized
          setBrief(null)
          setDates([])
          toast.error(res.message || t('Failed to load dashboard'))
        }
      } catch {
        if (requestId !== requestIdRef.current) return
        setBrief(null)
        setDates([])
        toast.error(t('Failed to load dashboard'))
      } finally {
        if (requestId === requestIdRef.current) {
          setLoading(false)
        }
      }
    },
    [userId, t]
  )

  useEffect(() => {
    if (open) {
      setRangeDays(7)
      fetchDashboard(7)
    } else {
      // Invalidate any in-flight request when the dialog closes.
      requestIdRef.current++
      setBrief(null)
      setDates([])
    }
  }, [open, fetchDashboard])

  const spec = useMemo(() => {
    const colors = getDashboardChartColors(dates.length || 1)
    // Aggregate by hourly bucket: quota + token_used per created_at
    const buckets = new Map<number, { quota: number; tokens: number }>()
    for (const d of dates) {
      const b = buckets.get(d.created_at) ?? { quota: 0, tokens: 0 }
      b.quota += d.quota ?? 0
      b.tokens += d.token_used ?? 0
      buckets.set(d.created_at, b)
    }
    const values = [...buckets.entries()]
      .sort((a, b) => a[0] - b[0])
      .map(([ts, v]) => ({
        Time: formatChartTime(ts, 'day'),
        rawQuota: v.quota,
        Tokens: v.tokens,
      }))
    return {
      // Common chart: quota as bars plus the token-usage line on the same
      // Time axis. Common charts need explicit axes (unlike single-series
      // specs) and a shared y-axis is fine for this compact modal.
      type: 'common',
      data: [{ id: 'quotaTrendData', values }],
      series: [
        { type: 'bar', xField: 'Time', yField: 'rawQuota' },
        { type: 'line', xField: 'Time', yField: 'Tokens' },
      ],
      axes: [
        { orient: 'bottom', type: 'band' },
        { orient: 'left', type: 'linear' },
      ],
      title: {
        visible: true,
        text: t('Quota Usage Trend'),
        subtext: values.length === 0 ? t('No data available') : undefined,
      },
      color: { type: 'ordinal', range: colors },
      background: { fill: 'transparent' },
    }
  }, [dates, t])

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={
        <>
          <ChartColumnBig className='h-5 w-5' />
          {t('Data Dashboard')}
        </>
      }
      description={t('{{username}} (ID: {{id}})', { username, id: userId })}
      contentClassName='sm:max-w-2xl'
      contentHeight='auto'
      bodyClassName='space-y-4'
    >
      <div className='flex items-center justify-between gap-2'>
        <div className='flex gap-2'>
          {RANGE_PRESETS.map((preset) => (
            <Button
              key={preset.days}
              variant={rangeDays === preset.days ? 'default' : 'outline'}
              size='sm'
              aria-pressed={rangeDays === preset.days}
              onClick={() => {
                setRangeDays(preset.days)
                fetchDashboard(preset.days)
              }}
            >
              {t(preset.labelKey)}
            </Button>
          ))}
        </div>
        {brief && (
          <p className='text-muted-foreground text-xs'>
            {t('Quota')}: {formatQuota(brief.quota)} · {t('Used Quota')}:{' '}
            {formatQuota(brief.used_quota)} · {t('Request Count')}:{' '}
            {brief.request_count}
          </p>
        )}
      </div>
      <div className='relative h-[300px]'>
        {loading && (
          // Overlay keeps the range controls usable while a refetch runs.
          <div className='absolute inset-0 z-10 flex items-center justify-center'>
            <Loader2 className='text-muted-foreground h-6 w-6 animate-spin' />
          </div>
        )}
        {themeReady && (
          <VChart
            key={`user-dashboard-${resolvedTheme}`}
            spec={{
              ...spec,
              theme: resolvedTheme === 'dark' ? 'dark' : 'light',
              background: 'transparent',
            }}
            option={VCHART_OPTION}
          />
        )}
      </div>
    </Dialog>
  )
}
