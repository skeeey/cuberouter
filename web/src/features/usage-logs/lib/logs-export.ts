/*
Copyright (C) 2023-2026 QuantumNous
Copyright (C) 2026 CubeRouter

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
import { getCurrencyLabel } from '@/lib/currency'
import dayjs from '@/lib/dayjs'
import { writeExcelSheet, type ExcelSheet } from '@/lib/excel'
import { quotaUnitsToDollars } from '@/lib/format'

import type { UsageLog } from '../data/schema'
import type { LogOtherData } from '../types'
import { parseLogOther } from './format'
import { getLogTypeConfig } from './utils'

/** How many rows a usage-log export may assemble. */
export const USAGE_LOGS_EXPORT_MAX_ROWS = 10_000

/** The backend clamps `page_size` to 100, so an export walks pages of 100. */
export const USAGE_LOGS_EXPORT_PAGE_SIZE = 100

const TOKEN_FORMAT = '#,##0'
const MONEY_FORMAT = '#,##0.0000'

export type UsageCategory = 'Text' | 'Image' | 'Video' | 'Audio' | 'Other'

export interface CommonLogsExportPage {
  items?: UsageLog[]
  total?: number
  page_size?: number
}

export interface CollectedCommonLogRows {
  items: UsageLog[]
  /** The endpoint's own count of the whole filtered set. */
  total: number
  /** The set was larger than the cap, so `items` is a leading slice of it. */
  truncated: boolean
}

function tokenCell(value: number) {
  return { value, type: Number, format: TOKEN_FORMAT }
}

function moneyCell(value: number) {
  return { value, type: Number, format: MONEY_FORMAT }
}

function headerCell(value: string) {
  return { value, fontWeight: 'bold' as const }
}

/**
 * The export groups rows by the modality the user cares about, not by billing
 * mode: video and image tasks are billed by seconds/size while chat is billed
 * by tokens, and a flat token view hides the former.
 */
function usageCategory(requestPath: string): UsageCategory {
  const path = requestPath.toLowerCase()
  if (path.includes('/audio/')) return 'Audio'
  if (path.includes('/image')) return 'Image'
  // Task-plugin channels submit through /v1/tasks/<plugin>; every task plugin
  // on this site is a video model.
  if (path.includes('/video/') || path.includes('/tasks/')) return 'Video'
  if (
    path === '' ||
    path.includes('/chat/') ||
    path.includes('/messages') ||
    path.includes('/responses') ||
    path.includes('/completions') ||
    path.includes('/embeddings') ||
    path.includes('/rerank')
  ) {
    return 'Text'
  }
  return 'Other'
}

function cacheWriteTokens(other: LogOtherData | null): number {
  const split =
    (other?.cache_creation_tokens_5m ?? 0) +
    (other?.cache_creation_tokens_1h ?? 0)
  return split > 0 ? split : (other?.cache_creation_tokens ?? 0)
}

/**
 * The quantity a user should read for this row.
 *
 * Video seconds and image size live in the log's `content`; audio has no such
 * sentence, so TTS rows report their character counts from `other` and speech
 * rows fall back to the character count the backend stores in prompt tokens.
 */
function usageDetails(
  content: string,
  other: LogOtherData | null,
  category: UsageCategory,
  promptTokens: number,
  t: (key: string) => string
): string {
  if (category === 'Audio') {
    const audioOutput = other?.audio_output ?? 0
    if (audioOutput > 0) return `${t('Audio Characters')}: ${audioOutput}`
    if (
      other?.request_path?.toLowerCase().includes('/audio/speech') &&
      promptTokens > 0
    ) {
      return `${t('Billing Characters')}: ${promptTokens}`
    }
  }

  if (content) return content

  const facts = other?.usage_facts
  if (facts && Object.keys(facts).length > 0) {
    return Object.entries(facts)
      .map(([key, value]) => `${key}=${value}`)
      .join(', ')
  }
  return ''
}

function channelLabel(row: UsageLog): string {
  if (!row.channel) return ''
  return row.channel_name
    ? `${row.channel_name} #${row.channel}`
    : `#${row.channel}`
}

/**
 * The spreadsheet for the common-logs table: one row per log record, all rows
 * matching the current filters, with the identity columns an administrator
 * needs and the usage columns a user needs. Admin-only columns are omitted
 * for self reads, mirroring what the table already shows.
 */
export function buildCommonLogsExportSheet(
  rows: UsageLog[],
  isAdmin: boolean,
  t: (key: string) => string
): ExcelSheet {
  const currency = getCurrencyLabel()
  const costHeader = `${t('Cost')} (${currency})`

  const header = [
    t('Time'),
    t('Log Type'),
    t('Usage Category'),
    ...(isAdmin ? [t('User'), t('Channel')] : []),
    t('Model'),
    ...(isAdmin ? [t('Upstream Model')] : []),
    t('Token'),
    t('Group'),
    t('Prompt Tokens'),
    t('Completion Tokens'),
    t('Cache Read Tokens'),
    t('Cache Write Tokens'),
    t('Usage Details'),
    costHeader,
    t('Request ID'),
    t('Upstream Request ID'),
    t('Task ID'),
  ].map(headerCell)

  const data = rows.map((row) => {
    const other = parseLogOther(row.other)
    const category = usageCategory(other?.request_path ?? '')
    return [
      dayjs(row.created_at * 1000).format('YYYY-MM-DD HH:mm:ss'),
      t(getLogTypeConfig(row.type).label),
      t(category),
      ...(isAdmin ? [row.username || '', channelLabel(row)] : []),
      row.model_name || '',
      ...(isAdmin ? [other?.upstream_model_name || ''] : []),
      row.token_name || '',
      row.group || '',
      tokenCell(row.prompt_tokens || 0),
      tokenCell(row.completion_tokens || 0),
      tokenCell(other?.cache_tokens || 0),
      tokenCell(cacheWriteTokens(other)),
      usageDetails(row.content, other, category, row.prompt_tokens || 0, t),
      moneyCell(quotaUnitsToDollars(row.quota || 0)),
      row.request_id || '',
      row.upstream_request_id || '',
      other?.task_id || '',
    ]
  })

  const columns = [
    { width: 20 },
    { width: 12 },
    { width: 14 },
    ...(isAdmin ? [{ width: 18 }, { width: 20 }] : []),
    { width: 22 },
    ...(isAdmin ? [{ width: 24 }] : []),
    { width: 20 },
    { width: 14 },
    { width: 14 },
    { width: 14 },
    { width: 16 },
    { width: 16 },
    { width: 36 },
    { width: 16 },
    { width: 22 },
    { width: 24 },
    { width: 24 },
  ]

  return {
    rows: [header, ...data],
    columns,
    filename: `usage-logs-${dayjs().format('YYYY-MM-DD-HHmmss')}.xlsx`,
  }
}

/** Writes the sheet through the browser build, which triggers a download. */
export async function writeCommonLogsExportFile(
  sheet: ExcelSheet
): Promise<void> {
  await writeExcelSheet(sheet)
}

/**
 * Pins the export window before the first page is fetched.
 *
 * The page endpoints derive "now + 1 hour" on every request when the URL has no
 * explicit range, so a multi-page export would otherwise walk a window that
 * moves forward and shift offsets between pages. Filling the missing bounds and
 * capping the end at the click instant also keeps new arrivals out of the set.
 */
export function pinLogsExportTimeRange(
  searchParams: Record<string, unknown>,
  now = Date.now()
): Record<string, unknown> {
  const startTime =
    typeof searchParams.startTime === 'number'
      ? searchParams.startTime
      : new Date(now).setHours(0, 0, 0, 0)
  const endTime =
    typeof searchParams.endTime === 'number'
      ? Math.min(searchParams.endTime, now)
      : now
  return { ...searchParams, startTime, endTime }
}

/**
 * Reads the whole filtered set one page at a time.
 *
 * `page_size` is clamped to 100 by the backend, so an export of a busy period
 * has to be assembled. The walk stops on the first page shorter than the size
 * the response reports, or once the endpoint's total is reached; more than
 * `maxRows` is reported as a truncated leading slice rather than held open.
 */
export async function collectCommonLogRows(
  fetchPage: (
    page: number,
    pageSize: number
  ) => Promise<CommonLogsExportPage | undefined>,
  options: { maxRows?: number } = {}
): Promise<CollectedCommonLogRows> {
  const maxRows = options.maxRows ?? USAGE_LOGS_EXPORT_MAX_ROWS
  const pageSize = USAGE_LOGS_EXPORT_PAGE_SIZE
  const items: UsageLog[] = []
  let total = 0
  let page = 1

  while (items.length < maxRows) {
    const result = (await fetchPage(page, pageSize)) ?? {}
    total = result.total ?? 0
    const batch = result.items ?? []
    items.push(...batch.slice(0, maxRows - items.length))
    const reportedPageSize = result.page_size ?? 0
    const answered = reportedPageSize > 0 ? reportedPageSize : pageSize
    if (batch.length < answered) break
    if (total > 0 && items.length >= total) break
    page += 1
  }

  return { items, total, truncated: total > items.length }
}
