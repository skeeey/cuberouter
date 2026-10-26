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
import type { ColumnDef } from '@tanstack/react-table'

import { StatusBadge } from '@/components/status-badge'
import { taskActionMapper } from '@/features/usage-logs/lib/mappers'
import { formatQuota } from '@/lib/format'

import {
  organizationTaskResponsibleName,
  organizationTaskTokenName,
  type Translate,
} from '../lib'

/**
 * A row these columns can describe.
 *
 * Stated structurally rather than as one of the two task rows, because both task
 * tables add the same columns over two different records. Every field here
 * exists on both.
 */
export type OrganizationTaskColumnRow = {
  task_id?: string
  action?: string
  responsible_name?: string
  creator_name?: string
  responsible_user_id?: number
  creator_user_id?: number
  user_id?: number
  token_name?: string
  token_id?: number
  quota?: number
  original_quota?: number
}

/**
 * The columns the organization adds to a task table.
 *
 * The shared task columns come from the usage-logs feature and describe the task
 * itself. None of them can say *whose* work it was, what it was, or what it
 * cost, because on a personal page those answers are already implied by who is
 * looking. Here they are not.
 *
 * The task id and type are optional because the Midjourney table has its own
 * identifier and action set; the async task table turns both on.
 */
export function buildOrganizationTaskColumns<
  T extends OrganizationTaskColumnRow,
>(options: {
  /** The caller reads every member's tasks, not just their own. */
  showMember: boolean
  /** Show the upstream task id, which is also the task ID search key. */
  showTaskId?: boolean
  /** Show the task action, e.g. Image to Video. */
  showType?: boolean
  translate: Translate
}): ColumnDef<T>[] {
  const { translate } = options
  const columns: ColumnDef<T>[] = []

  if (options.showTaskId) {
    columns.push({
      id: 'task_id',
      header: translate('Task ID'),
      size: 210,
      cell: ({ row }) => {
        const taskId = row.original.task_id
        if (!taskId) {
          return <span className='text-muted-foreground/60 text-xs'>-</span>
        }
        return (
          <StatusBadge
            label={taskId}
            copyText={taskId}
            variant='neutral'
            size='sm'
            className='border-border/60 bg-muted/30 !text-foreground max-w-full truncate rounded-md border px-1.5 py-0.5 font-mono'
          />
        )
      },
    })
  }

  if (options.showType) {
    columns.push({
      id: 'action',
      header: translate('Type'),
      size: 150,
      cell: ({ row }) => {
        const action = row.original.action
        if (!action) {
          return <span className='text-muted-foreground/60 text-xs'>-</span>
        }
        return (
          <span className='text-sm whitespace-nowrap'>
            {translate(taskActionMapper.getLabel(action))}
          </span>
        )
      },
    })
  }

  if (options.showMember) {
    columns.push({
      id: 'member',
      header: translate('Member'),
      size: 150,
      cell: ({ row }) => (
        <span className='text-sm whitespace-nowrap'>
          {organizationTaskResponsibleName(row.original)}
        </span>
      ),
    })
  }

  columns.push(
    {
      id: 'token_name',
      header: translate('Key'),
      size: 150,
      cell: ({ row }) => (
        <span className='font-mono text-xs'>
          {organizationTaskTokenName(row.original)}
        </span>
      ),
    },
    {
      id: 'original_quota',
      header: translate('Original price'),
      size: 110,
      cell: ({ row }) => (
        // The original charge is the same as the charged amount for rows
        // written before the billing context existed, or for a task that never
        // cost anything.
        <span className='tabular-nums'>
          {formatQuota(row.original.original_quota ?? row.original.quota ?? 0)}
        </span>
      ),
    },
    {
      id: 'quota',
      header: translate('Discounted price'),
      size: 110,
      cell: ({ row }) => (
        <span className='tabular-nums'>
          {formatQuota(row.original.quota ?? 0)}
        </span>
      ),
    }
  )

  return columns
}
