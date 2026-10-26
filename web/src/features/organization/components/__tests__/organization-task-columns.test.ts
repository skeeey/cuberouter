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

import type { OrganizationTaskLog } from '../../lib'
import { buildOrganizationTaskColumns } from '../organization-task-columns'

function translate(key: string): string {
  return key
}

describe('organization task columns', () => {
  test('adds task id, type, member, key and split prices', () => {
    const columns = buildOrganizationTaskColumns<OrganizationTaskLog>({
      showMember: true,
      showTaskId: true,
      showType: true,
      translate,
    })

    expect(columns.map((column) => column.id)).toEqual([
      'task_id',
      'action',
      'member',
      'token_name',
      'original_quota',
      'quota',
    ])
    expect(columns.map((column) => column.header)).toEqual([
      'Task ID',
      'Type',
      'Member',
      'Key',
      'Original price',
      'Discounted price',
    ])
  })

  test('omits the member column for a caller who cannot see wide data', () => {
    const columns = buildOrganizationTaskColumns<OrganizationTaskLog>({
      showMember: false,
      showTaskId: true,
      showType: true,
      translate,
    })

    expect(columns.map((column) => column.id)).toEqual([
      'task_id',
      'action',
      'token_name',
      'original_quota',
      'quota',
    ])
  })
})
