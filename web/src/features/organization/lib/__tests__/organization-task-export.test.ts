/*
Copyright (C) 2023-2026 QuantumNous
Copyright (C) 2026 CubeRouter

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY OR FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { describe, expect, test } from 'vitest'

import { getCurrencyLabel } from '@/lib/currency'
import dayjs from '@/lib/dayjs'
import { formatTimestamp, quotaUnitsToDollars } from '@/lib/format'

import type { OrganizationTaskLog } from '../organization-task'
import { buildTaskExportSheet } from '../organization-task-export'

function cellValue(cell: unknown): unknown {
  if (cell && typeof cell === 'object' && 'value' in cell) {
    return (cell as { value?: unknown }).value
  }
  return cell
}

function task(
  overrides: Partial<OrganizationTaskLog> = {}
): OrganizationTaskLog {
  return {
    id: 1,
    user_id: 42,
    platform: 'mock-platform',
    task_id: 'task-1',
    action: 'generate',
    channel_id: 2,
    group: 'default',
    quota: 800_000,
    original_quota: 1_000_000,
    submit_time: dayjs('2026-10-20T10:00:00').unix(),
    finish_time: dayjs('2026-10-20T10:01:30').unix(),
    status: 'SUCCESS',
    token_name: 'key-1',
    responsible_user_id: 42,
    responsible_name: 'Alice',
    creator_user_id: 42,
    creator_name: 'Alice',
    ...overrides,
  }
}

function translate(key: string): string {
  return key
}

describe('buildTaskExportSheet', () => {
  test('detail mode exports the visible task columns', () => {
    const currency = getCurrencyLabel()
    const sheet = buildTaskExportSheet([task()], 'detail', translate)
    const header = sheet.rows[0].map(cellValue)
    const values = sheet.rows[1].map(cellValue)

    expect(header).toEqual([
      'Submit Time',
      'Task ID',
      'Type',
      'Member',
      'Key',
      `Original price (${currency})`,
      `Discounted price (${currency})`,
      'Duration (s)',
      'Status',
    ])
    expect(values).toEqual([
      formatTimestamp(task().submit_time),
      'task-1',
      'Image to Video',
      'Alice',
      'key-1',
      quotaUnitsToDollars(1_000_000),
      quotaUnitsToDollars(800_000),
      90,
      'Success',
    ])
  })

  test('daily mode aggregates tasks, terminal states and amounts per day', () => {
    const currency = getCurrencyLabel()
    const sheet = buildTaskExportSheet(
      [
        task({ id: 1 }),
        task({
          id: 2,
          task_id: 'task-2',
          status: 'FAILURE',
          quota: 0,
          original_quota: 0,
          finish_time: dayjs('2026-10-20T11:00:00').unix(),
        }),
        task({
          id: 3,
          task_id: 'task-3',
          submit_time: dayjs('2026-10-21T09:00:00').unix(),
          finish_time: dayjs('2026-10-21T09:02:00').unix(),
          quota: 400_000,
          original_quota: 500_000,
        }),
      ],
      'daily',
      translate
    )

    expect(sheet.rows[0].map(cellValue)).toEqual([
      'Date',
      'Tasks',
      'Succeeded',
      'Failed',
      `Original price (${currency})`,
      `Discounted price (${currency})`,
    ])
    expect(sheet.rows[1].map(cellValue)).toEqual([
      '2026-10-20',
      2,
      1,
      1,
      quotaUnitsToDollars(1_000_000),
      quotaUnitsToDollars(800_000),
    ])
    expect(sheet.rows[2].map(cellValue)).toEqual([
      '2026-10-21',
      1,
      1,
      0,
      quotaUnitsToDollars(500_000),
      quotaUnitsToDollars(400_000),
    ])
  })
})
