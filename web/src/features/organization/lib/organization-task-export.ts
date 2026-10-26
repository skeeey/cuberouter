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
import {
  taskActionMapper,
  taskStatusMapper,
} from '@/features/usage-logs/lib/mappers'
import { getCurrencyLabel } from '@/lib/currency'
import dayjs from '@/lib/dayjs'
import { writeExcelSheet, type ExcelSheet } from '@/lib/excel'
import { formatTimestamp, quotaUnitsToDollars } from '@/lib/format'

import {
  organizationTaskResponsibleName,
  organizationTaskTokenName,
  type OrganizationTaskLog,
} from './organization-task'

export type TaskExportMode = 'detail' | 'daily'

const NUMBER_FORMAT = '#,##0'
const MONEY_FORMAT = '#,##0.0000'

function numberCell(value: number) {
  return { value, type: Number, format: NUMBER_FORMAT }
}

function moneyCell(value: number) {
  return { value, type: Number, format: MONEY_FORMAT }
}

function headerCell(value: string) {
  return { value, fontWeight: 'bold' as const }
}

/**
 * The spreadsheet for the task table.
 *
 * The detail sheet mirrors the visible columns; the daily sheet keeps the date,
 * the count by terminal state and the two amounts, because that is the shape
 * the reader asked for. Duration is exported in whole seconds so a spreadsheet
 * can sum it.
 */
export function buildTaskExportSheet(
  rows: OrganizationTaskLog[],
  mode: TaskExportMode,
  t: (key: string) => string
): ExcelSheet {
  const currency = getCurrencyLabel()
  const originalHeader = `${t('Original price')} (${currency})`
  const discountedHeader = `${t('Discounted price')} (${currency})`

  if (mode === 'daily') {
    const days = new Map<
      string,
      {
        tasks: number
        succeeded: number
        failed: number
        original: number
        discounted: number
      }
    >()

    for (const row of rows) {
      const seconds = row.submit_time || row.created_at || 0
      const date =
        seconds > 0 ? dayjs(seconds * 1000).format('YYYY-MM-DD') : t('Unknown')
      const day = days.get(date) ?? {
        tasks: 0,
        succeeded: 0,
        failed: 0,
        original: 0,
        discounted: 0,
      }
      day.tasks += 1
      if (row.status === 'SUCCESS') day.succeeded += 1
      if (row.status === 'FAILURE') day.failed += 1
      day.original += row.original_quota ?? row.quota ?? 0
      day.discounted += row.quota ?? 0
      days.set(date, day)
    }

    return {
      rows: [
        [
          headerCell(t('Date')),
          headerCell(t('Tasks')),
          headerCell(t('Succeeded')),
          headerCell(t('Failed')),
          headerCell(originalHeader),
          headerCell(discountedHeader),
        ],
        ...[...days.entries()]
          .sort(([left], [right]) => left.localeCompare(right))
          .map(([date, day]) => [
            date,
            numberCell(day.tasks),
            numberCell(day.succeeded),
            numberCell(day.failed),
            moneyCell(quotaUnitsToDollars(day.original)),
            moneyCell(quotaUnitsToDollars(day.discounted)),
          ]),
      ],
      columns: [
        { width: 14 },
        { width: 10 },
        { width: 12 },
        { width: 10 },
        { width: 18 },
        { width: 18 },
      ],
      filename: taskExportFilename('daily'),
    }
  }

  return {
    rows: [
      [
        headerCell(t('Submit Time')),
        headerCell(t('Task ID')),
        headerCell(t('Type')),
        headerCell(t('Member')),
        headerCell(t('Key')),
        headerCell(originalHeader),
        headerCell(discountedHeader),
        headerCell(t('Duration (s)')),
        headerCell(t('Status')),
      ],
      ...rows.map((row) => {
        const duration =
          row.submit_time && row.finish_time
            ? Math.max(0, row.finish_time - row.submit_time)
            : 0
        return [
          row.submit_time ? formatTimestamp(row.submit_time) : '',
          row.task_id || '',
          row.action ? t(taskActionMapper.getLabel(row.action)) : '',
          organizationTaskResponsibleName(row),
          organizationTaskTokenName(row),
          moneyCell(quotaUnitsToDollars(row.original_quota ?? row.quota ?? 0)),
          moneyCell(quotaUnitsToDollars(row.quota ?? 0)),
          numberCell(duration),
          row.status
            ? t(taskStatusMapper.getLabel(row.status, row.status))
            : '',
        ]
      }),
    ],
    columns: [
      { width: 20 },
      { width: 22 },
      { width: 18 },
      { width: 16 },
      { width: 18 },
      { width: 18 },
      { width: 18 },
      { width: 12 },
      { width: 12 },
    ],
    filename: taskExportFilename('detail'),
  }
}

/** Writes the sheet through the browser build, which triggers a download. */
export async function writeTaskExportFile(sheet: ExcelSheet): Promise<void> {
  await writeExcelSheet(sheet)
}

function taskExportFilename(mode: TaskExportMode, date = new Date()): string {
  return `organization-tasks-${mode}-${dayjs(date).format('YYYY-MM-DD-HHmmss')}.xlsx`
}
