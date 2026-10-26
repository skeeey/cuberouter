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
import { describe, expect, test } from 'vitest'

import { getCurrencyLabel } from '@/lib/currency'
import dayjs from '@/lib/dayjs'
import { quotaUnitsToDollars } from '@/lib/format'

import type { OrganizationLogRow } from '../../types'
import { buildUsageExportSheet } from '../organization-usage-export'

function cellValue(cell: unknown): unknown {
  if (cell && typeof cell === 'object' && 'value' in cell) {
    return (cell as { value?: unknown }).value
  }
  return cell
}

function row(overrides: Partial<OrganizationLogRow> = {}): OrganizationLogRow {
  return {
    id: 1,
    user_id: 42,
    created_at: dayjs('2026-10-20T10:00:00').unix(),
    type: 2,
    token_name: 'key-1',
    model_name: 'gpt-4o',
    quota: 800_000,
    prompt_tokens: 1_000,
    completion_tokens: 500,
    use_time: 1,
    is_stream: false,
    responsible_user_id: 42,
    responsible_name: 'Alice',
    other: JSON.stringify({
      group_ratio: 0.8,
      model_ratio: 1.2,
      cache_tokens: 300,
      is_model_mapped: true,
      upstream_model_name: 'gpt-4o-upstream',
    }),
    ...overrides,
  }
}

function translate(key: string): string {
  return key
}

describe('buildUsageExportSheet', () => {
  test('detail mode exports the visible columns one row per request', () => {
    const currency = getCurrencyLabel()
    const sheet = buildUsageExportSheet([row()], 'detail', translate)
    const header = sheet.rows[0].map(cellValue)
    const values = sheet.rows[1].map(cellValue)

    expect(header).toEqual([
      'User',
      'API Key Name',
      'Model',
      'Upstream',
      'Billing Mode',
      'Prompt Tokens',
      'Completion Tokens',
      'Cache Read Tokens',
      `Original price (${currency})`,
      `User charge (${currency})`,
      'Time',
    ])
    expect(values).toEqual([
      'Alice',
      'key-1',
      'gpt-4o',
      'gpt-4o-upstream',
      'Ratio',
      1_000,
      500,
      300,
      quotaUnitsToDollars(1_000_000),
      quotaUnitsToDollars(800_000),
      '2026-10-20 10:00:00',
    ])
  })

  test('daily mode aggregates each local day into one row', () => {
    const currency = getCurrencyLabel()
    const sheet = buildUsageExportSheet(
      [
        row({ id: 1 }),
        row({
          id: 2,
          created_at: dayjs('2026-10-20T18:00:00').unix(),
          quota: 400_000,
          prompt_tokens: 2_000,
          completion_tokens: 1_000,
          other: JSON.stringify({
            group_ratio: 1,
            model_ratio: 1.2,
            cache_tokens: 200,
          }),
        }),
        row({
          id: 3,
          created_at: dayjs('2026-10-21T09:00:00').unix(),
          quota: 200_000,
          prompt_tokens: 500,
          completion_tokens: 100,
          other: JSON.stringify({ group_ratio: 1, model_ratio: 1 }),
        }),
      ],
      'daily',
      translate
    )

    expect(sheet.rows[0].map(cellValue)).toEqual([
      'Date',
      'Requests',
      'Prompt Tokens',
      'Completion Tokens',
      'Cache Read Tokens',
      `Original price (${currency})`,
      `User charge (${currency})`,
    ])
    expect(sheet.rows[1].map(cellValue)).toEqual([
      '2026-10-20',
      2,
      3_000,
      1_500,
      500,
      quotaUnitsToDollars(1_000_000 + 400_000),
      quotaUnitsToDollars(800_000 + 400_000),
    ])
    expect(sheet.rows[2].map(cellValue)).toEqual([
      '2026-10-21',
      1,
      500,
      100,
      0,
      quotaUnitsToDollars(200_000),
      quotaUnitsToDollars(200_000),
    ])
  })
})
