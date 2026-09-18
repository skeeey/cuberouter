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
import { useQuery } from '@tanstack/react-query'
import { HeartPulse, Timer } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { getPerfMetrics } from '@/features/performance-metrics/api'
import {
  formatLatency,
  formatThroughput,
  formatUptimePct,
  getSuccessRateTextClass,
} from '@/features/performance-metrics/lib/format'
import type { PerformanceGroup } from '@/features/performance-metrics/types'
import { cn } from '@/lib/utils'

import type { UptimeDayPoint } from '../lib/mock-stats'
import type { PricingModel } from '../types'
import {
  LatencyTrendChart,
  UptimeTrendChart,
  type LatencyTrendPoint,
} from './model-details-charts'

function StatCard(props: {
  icon: React.ComponentType<{ className?: string }>
  label: string
  value: React.ReactNode
  hint?: string
  valueClassName?: string
}) {
  const Icon = props.icon
  return (
    <div className='bg-background flex flex-col gap-1 rounded-lg border p-3'>
      <span className='text-muted-foreground inline-flex items-center gap-1.5 text-[10px] font-medium tracking-wider uppercase'>
        <Icon className='size-3' aria-hidden='true' />
        {props.label}
      </span>
      <span
        className={cn(
          'text-foreground font-mono text-lg font-semibold tabular-nums',
          props.valueClassName
        )}
      >
        {props.value}
      </span>
      {props.hint && (
        <span className='text-muted-foreground/70 text-[11px]'>
          {props.hint}
        </span>
      )}
    </div>
  )
}

type PerformanceRow = {
  group: string
  avg_ttft_ms: number
  avg_latency_ms: number
  success_rate: number
  avg_tps: number
}

function toUptimePct(value: number): number {
  if (!Number.isFinite(value)) return 0
  const clamped = Math.min(100, Math.max(0, value))
  return Math.round(clamped * 100) / 100
}

type WeightedSample = { value: number; weight: number }

// The model-level latency lines merge the per-group buckets of a model into
// one point per time bucket and metric (average TTFT, P95 latency, P99
// latency). Each bucket value is weighted by the owning group's request_count
// in that bucket, so the merged value is a request-weighted mean of the
// per-group values — an approximation; the exact model-level percentile
// requires merging the per-group latency histograms server-side (future
// work). Groups without a per-bucket request_count (older cached payloads)
// fall back to equal weight. Values that signal missing data (avg TTFT <= 0;
// percentile -1, the backend's no-data sentinel) produce no row for that
// metric, so the chart simply omits the point instead of plotting an empty
// value.
function toLatencyTrendSeries(groups: PerformanceGroup[]): LatencyTrendPoint[] {
  const byTs = new Map<
    number,
    { ttft: WeightedSample[]; p95: WeightedSample[]; p99: WeightedSample[] }
  >()
  for (const group of groups) {
    for (const point of group.series) {
      let entry = byTs.get(point.ts)
      if (!entry) {
        entry = { ttft: [], p95: [], p99: [] }
        byTs.set(point.ts, entry)
      }
      const weight = point.request_count ?? 1
      if (point.avg_ttft_ms > 0) {
        entry.ttft.push({ value: point.avg_ttft_ms, weight })
      }
      if (point.p95_latency_ms != null && point.p95_latency_ms >= 0) {
        entry.p95.push({ value: point.p95_latency_ms, weight })
      }
      if (point.p99_latency_ms != null && point.p99_latency_ms >= 0) {
        entry.p99.push({ value: point.p99_latency_ms, weight })
      }
    }
  }

  const series: LatencyTrendPoint[] = []
  for (const ts of [...byTs.keys()].sort((a, b) => a - b)) {
    const entry = byTs.get(ts)
    if (!entry) continue
    const timestamp = new Date(ts * 1000).toISOString()
    for (const metric of ['ttft', 'p95', 'p99'] as const) {
      const samples = entry[metric]
      if (samples.length === 0) continue
      series.push({
        timestamp,
        metric,
        ms: Math.round(weightedAverage(samples)),
      })
    }
  }
  return series
}

// Weighted mean of the samples. When every sample carries a zero request
// count the weights sum to nothing, so the samples fall back to an
// equal-weight mean instead of dropping the point.
function weightedAverage(samples: WeightedSample[]): number {
  const totalWeight = samples.reduce((sum, sample) => sum + sample.weight, 0)
  if (totalWeight <= 0) {
    return (
      samples.reduce((sum, sample) => sum + sample.value, 0) / samples.length
    )
  }
  return (
    samples.reduce((sum, sample) => sum + sample.value * sample.weight, 0) /
    totalWeight
  )
}

function toUptimeSeries(groups: PerformanceGroup[]): UptimeDayPoint[] {
  const byTs = new Map<number, { rates: number[]; incidents: number }>()
  for (const group of groups) {
    for (const point of group.series) {
      const current = byTs.get(point.ts) ?? { rates: [], incidents: 0 }
      if (Number.isFinite(point.success_rate)) {
        const successRate = toUptimePct(point.success_rate)
        current.rates.push(successRate)
        if (successRate < 100) current.incidents += 1
      }
      byTs.set(point.ts, current)
    }
  }
  return [...byTs.entries()]
    .sort(([a], [b]) => a - b)
    .map(([ts, value]) => {
      const uptime =
        value.rates.length > 0
          ? value.rates.reduce((sum, rate) => sum + rate, 0) /
            value.rates.length
          : 0
      return {
        date: new Date(ts * 1000).toISOString(),
        uptime_pct: toUptimePct(uptime),
        incidents: value.incidents,
        outage_minutes: 0,
      }
    })
}

function average(
  rows: PerformanceRow[],
  field: 'avg_ttft_ms' | 'avg_latency_ms'
) {
  const values = rows.map((row) => row[field]).filter((value) => value > 0)
  if (values.length === 0) return 0
  return Math.round(
    values.reduce((sum, value) => sum + value, 0) / values.length
  )
}

export function ModelDetailsPerformance(props: { model: PricingModel }) {
  const { t } = useTranslation()
  const metricsQuery = useQuery({
    queryKey: ['perf-metrics', props.model.model_name],
    queryFn: () => getPerfMetrics(props.model.model_name, 24),
    staleTime: 60 * 1000,
  })
  const groups = useMemo(
    () => metricsQuery.data?.data.groups ?? [],
    [metricsQuery.data]
  )
  const performances = useMemo<PerformanceRow[]>(
    () =>
      groups.map((group) => ({
        group: group.group,
        avg_ttft_ms: group.avg_ttft_ms,
        avg_latency_ms: group.avg_latency_ms,
        success_rate: group.success_rate,
        avg_tps: group.avg_tps,
      })),
    [groups]
  )
  const latencySeries = useMemo(() => toLatencyTrendSeries(groups), [groups])
  const uptimeSeries = useMemo(() => toUptimeSeries(groups), [groups])

  if (metricsQuery.isLoading || performances.length === 0) {
    return (
      <div className='text-muted-foreground rounded-lg border p-6 text-center text-sm'>
        {t('Performance data is not yet available for this model.')}
      </div>
    )
  }

  const tpsValues = performances
    .map((p) => p.avg_tps)
    .filter((value) => value > 0)
  const avgTps =
    tpsValues.length > 0
      ? tpsValues.reduce((sum, value) => sum + value, 0) / tpsValues.length
      : 0
  const avgLatency = average(performances, 'avg_latency_ms')
  const avgTtft = average(performances, 'avg_ttft_ms')
  const successRates = performances
    .map((perf) => perf.success_rate)
    .filter((value) => Number.isFinite(value))
  const successRate =
    successRates.length > 0
      ? successRates.reduce((sum, value) => sum + value, 0) /
        successRates.length
      : 0

  return (
    <div className='flex flex-col gap-4'>
      <div className='grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-4'>
        <StatCard
          icon={Timer}
          label='TPS'
          value={formatThroughput(avgTps)}
          hint={t('Sustained tokens per second')}
        />
        <StatCard
          icon={Timer}
          label={t('Average latency')}
          value={formatLatency(avgLatency)}
        />
        <StatCard
          icon={Timer}
          label={t('Average TTFT')}
          value={formatLatency(avgTtft)}
        />
        <StatCard
          icon={HeartPulse}
          label={t('Success rate')}
          value={formatUptimePct(successRate)}
          valueClassName={getSuccessRateTextClass(successRate)}
        />
      </div>

      <section>
        <SectionHeader
          icon={Timer}
          title={t('Latency trend (last 24h)')}
          description={t('Average TTFT with P95/P99 latency')}
        />
        <LatencyTrendChart series={latencySeries} />
      </section>

      <section>
        <SectionHeader
          icon={HeartPulse}
          title={t('Availability (last 24h)')}
        />
        <UptimeTrendChart series={uptimeSeries} />
      </section>
    </div>
  )
}

function SectionHeader(props: {
  icon: React.ComponentType<{ className?: string }>
  title: string
  description?: string
  accent?: React.ReactNode
}) {
  const Icon = props.icon
  return (
    <div className='mb-2 flex flex-wrap items-center justify-between gap-2'>
      <div className='flex min-w-0 items-center gap-2'>
        <Icon className='text-muted-foreground/70 size-3.5 shrink-0' />
        <div className='min-w-0'>
          <div className='text-foreground text-sm font-semibold'>
            {props.title}
          </div>
          {props.description && (
            <p className='text-muted-foreground/80 text-xs'>
              {props.description}
            </p>
          )}
        </div>
      </div>
      {props.accent && (
        <div className='shrink-0 text-xs font-medium'>{props.accent}</div>
      )}
    </div>
  )
}
