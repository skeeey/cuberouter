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
import { parseLogOther } from '@/features/usage-logs/lib/format'
import { getCurrencyLabel } from '@/lib/currency'
import dayjs from '@/lib/dayjs'
import { writeExcelSheet, type ExcelSheet } from '@/lib/excel'
import { quotaUnitsToDollars } from '@/lib/format'

import type { OrganizationLogRow } from '../types'
import {
  organizationLogBillingMode,
  organizationLogModelInfo,
  organizationLogOriginalQuota,
  organizationLogResponsibleName,
  organizationLogTokenName,
  ORGANIZATION_LOG_BILLING_MODE_LABELS,
} from './organization-log'

export type UsageExportMode = 'detail' | 'daily'

export type UsageExportSheet = ExcelSheet

const TOKEN_FORMAT = '#,##0'
const MONEY_FORMAT = '#,##0.0000'

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
 * The spreadsheet for the Usage table.
 *
 * The detail sheet mirrors the table, one row per request. The daily sheet
 * keeps only the date and the sums, because a day is the unit the reader asked
 * for; the identity columns are deliberately dropped rather than repeated for
 * one arbitrary row of the day.
 */
export function buildUsageExportSheet(
  rows: OrganizationLogRow[],
  mode: UsageExportMode,
  t: (key: string) => string
): UsageExportSheet {
  const currency = getCurrencyLabel()
  const originalHeader = `${t('Original price')} (${currency})`
  const chargeHeader = `${t('User charge')} (${currency})`

  if (mode === 'daily') {
    const days = new Map<
      string,
      {
        requests: number
        prompt: number
        completion: number
        cache: number
        original: number
        charge: number
      }
    >()

    for (const row of rows) {
      const date = dayjs(row.created_at * 1000).format('YYYY-MM-DD')
      const day = days.get(date) ?? {
        requests: 0,
        prompt: 0,
        completion: 0,
        cache: 0,
        original: 0,
        charge: 0,
      }
      const other = parseLogOther(row.other ?? '')
      day.requests += 1
      day.prompt += row.prompt_tokens || 0
      day.completion += row.completion_tokens || 0
      day.cache += other?.cache_tokens || 0
      day.original += organizationLogOriginalQuota(row)
      day.charge += row.quota || 0
      days.set(date, day)
    }

    return {
      rows: [
        [
          headerCell(t('Date')),
          headerCell(t('Requests')),
          headerCell(t('Prompt Tokens')),
          headerCell(t('Completion Tokens')),
          headerCell(t('Cache Read Tokens')),
          headerCell(originalHeader),
          headerCell(chargeHeader),
        ],
        ...[...days.entries()]
          .sort(([left], [right]) => left.localeCompare(right))
          .map(([date, day]) => [
            date,
            tokenCell(day.requests),
            tokenCell(day.prompt),
            tokenCell(day.completion),
            tokenCell(day.cache),
            moneyCell(quotaUnitsToDollars(day.original)),
            moneyCell(quotaUnitsToDollars(day.charge)),
          ]),
      ],
      columns: [
        { width: 14 },
        { width: 10 },
        { width: 14 },
        { width: 14 },
        { width: 16 },
        { width: 18 },
        { width: 18 },
      ],
      filename: usageExportFilename('daily'),
    }
  }

  return {
    rows: [
      [
        headerCell(t('User')),
        headerCell(t('API Key Name')),
        headerCell(t('Model')),
        headerCell(t('Upstream')),
        headerCell(t('Billing Mode')),
        headerCell(t('Prompt Tokens')),
        headerCell(t('Completion Tokens')),
        headerCell(t('Cache Read Tokens')),
        headerCell(originalHeader),
        headerCell(chargeHeader),
        headerCell(t('Time')),
      ],
      ...rows.map((row) => {
        const other = parseLogOther(row.other ?? '')
        const billingMode = organizationLogBillingMode(other)
        return [
          organizationLogResponsibleName(row),
          organizationLogTokenName(row),
          row.model_name || '',
          organizationLogModelInfo(row).actualModel || '',
          billingMode
            ? t(ORGANIZATION_LOG_BILLING_MODE_LABELS[billingMode])
            : '',
          tokenCell(row.prompt_tokens || 0),
          tokenCell(row.completion_tokens || 0),
          tokenCell(other?.cache_tokens || 0),
          moneyCell(quotaUnitsToDollars(organizationLogOriginalQuota(row))),
          moneyCell(quotaUnitsToDollars(row.quota || 0)),
          dayjs(row.created_at * 1000).format('YYYY-MM-DD HH:mm:ss'),
        ]
      }),
    ],
    columns: [
      { width: 18 },
      { width: 20 },
      { width: 22 },
      { width: 24 },
      { width: 14 },
      { width: 14 },
      { width: 14 },
      { width: 16 },
      { width: 18 },
      { width: 18 },
      { width: 20 },
    ],
    filename: usageExportFilename('detail'),
  }
}

/** Writes the sheet through the browser build, which triggers a download. */
export async function writeUsageExportFile(
  sheet: UsageExportSheet
): Promise<void> {
  await writeExcelSheet(sheet)
}

function usageExportFilename(mode: UsageExportMode, date = new Date()): string {
  return `organization-usage-${mode}-${dayjs(date).format('YYYY-MM-DD-HHmmss')}.xlsx`
}
