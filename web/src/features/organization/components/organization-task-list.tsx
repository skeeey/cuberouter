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
import { ChevronDown, Download, Loader2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { DataTablePage, useDataTable } from '@/components/data-table'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { useTaskLogsColumns } from '@/features/usage-logs/components/columns/task-logs-columns'
import { CompactDateTimeRangePicker } from '@/features/usage-logs/components/compact-date-time-range-picker'
import {
  LogsFilterField,
  LogsFilterToolbar,
} from '@/features/usage-logs/components/logs-filter-toolbar'
import { getDefaultTimeRange } from '@/features/usage-logs/lib/utils'
import { useTableUrlState } from '@/hooks/use-table-url-state'

import { listOrganizationTasks } from '../api'
import { useOrganizationPagedSection } from '../hooks/use-organization-paged-query'
import {
  collectOrganizationRows,
  insertColumnsAfter,
  organizationColumnFilterValue,
  organizationLogTimeWindow,
  organizationTaskTimeRangeParams,
  organizationTaskToLog,
  ORGANIZATION_TASK_ACTION_OPTIONS,
  ORGANIZATION_TASK_STATUS_OPTIONS,
  setOrganizationTextFilter,
  type OrganizationTaskLog,
} from '../lib'
import { ORGANIZATION_DEFAULT_PAGE_SIZE } from '../lib/organization-pagination'
import {
  buildTaskExportSheet,
  writeTaskExportFile,
  type TaskExportMode,
} from '../lib/organization-task-export'
import type { OrganizationTaskRow } from '../types'
import {
  OrganizationSelectFilter,
  OrganizationTextFilter,
} from './organization-filter-fields'
import {
  useOrganizationSectionRoute,
  useOrganizationSurface,
} from './organization-page-provider'
import { buildOrganizationTaskColumns } from './organization-task-columns'

const TASK_COLUMN_VISIBILITY_STORAGE_KEY = 'organization-task-column-visibility'

type OrganizationTaskListProps = {
  organizationId: number
  /** The caller reads every member's tasks, not just their own. */
  canViewWideData: boolean
  /** Leaves the page when the backend refuses the read. */
  onForbidden: () => void
}

/**
 * The organization's async tasks.
 *
 * The table itself is the personal task table's, down to the dialog behind each
 * row: an organization task record *is* a task record, and the endpoint returns
 * the same rows with columns added to say whose work it was. Sharing the columns
 * is what keeps the two tables from drifting apart.
 *
 * Only the filters are the organization's own, because only the filters are the
 * endpoint's own. It takes a task id, a status and an action, and it pins the
 * list to the caller unless they may read everyone's — so there is no member
 * filter here: a member whose rows the caller cannot see is not something they
 * can search for, and offering the box would only make the request look more
 * specific than the answer is.
 */
export function OrganizationTaskList(props: OrganizationTaskListProps) {
  const { t } = useTranslation()
  const { search, navigate } = useOrganizationSectionRoute()
  const surface = useOrganizationSurface()

  const {
    columnFilters,
    onColumnFiltersChange,
    pagination,
    onPaginationChange,
    ensurePageInRange,
  } = useTableUrlState({
    search,
    navigate,
    pagination: {
      defaultPage: 1,
      defaultPageSize: ORGANIZATION_DEFAULT_PAGE_SIZE,
    },
    globalFilter: { enabled: false },
    columnFilters: [
      { columnId: 'task_id', searchKey: 'taskId', type: 'string' },
      { columnId: 'status', searchKey: 'taskStatus', type: 'string' },
      { columnId: 'action', searchKey: 'taskAction', type: 'string' },
    ],
  })

  const window = organizationLogTimeWindow(search, getDefaultTimeRange())
  // Seconds: this table stores `submit_time` in whole seconds
  // (`model.Task.SubmitTime` is `time.Now().Unix()`), and the endpoint compares
  // the parameter against the stored column without converting it, so a
  // millisecond window would quietly match nothing.
  const timeRange = organizationTaskTimeRangeParams(
    window.start,
    window.end,
    'seconds'
  )

  const filters = {
    task_id: organizationColumnFilterValue(columnFilters, 'task_id'),
    status: organizationColumnFilterValue(columnFilters, 'status'),
    action: organizationColumnFilterValue(columnFilters, 'action'),
  }

  const listParams = {
    p: pagination.pageIndex + 1,
    page_size: pagination.pageSize,
    ...filters,
    ...timeRange,
  }

  const tasks = useOrganizationPagedSection<OrganizationTaskRow>({
    surface,
    organizationId: props.organizationId,
    resource: 'tasks',
    params: listParams,
    query: () =>
      listOrganizationTasks(surface, props.organizationId, listParams),
    onForbidden: props.onForbidden,
  })

  // The personal task table's columns, minus the ones the organization view
  // does not show: the task id and type are re-added by the organization
  // columns in a more useful form, progress is usually empty for asynchronous
  // upstreams, and artifacts are not part of this page.
  const sharedColumns = useTaskLogsColumns(false, false).filter((column) => {
    const { id, accessorKey } = column as { id?: string; accessorKey?: string }
    const columnId = id ?? accessorKey
    return (
      columnId !== 'task_id' &&
      columnId !== 'progress' &&
      columnId !== 'artifacts'
    )
  })
  const organizationColumns = buildOrganizationTaskColumns<OrganizationTaskLog>(
    {
      showMember: props.canViewWideData,
      showTaskId: true,
      showType: true,
      translate: t,
    }
  )
  const columns = insertColumnsAfter(
    sharedColumns,
    'submit_time',
    organizationColumns
  )

  const rows = tasks.items.map(organizationTaskToLog)

  const [exporting, setExporting] = useState<TaskExportMode | null>(null)

  const handleExport = async (mode: TaskExportMode) => {
    setExporting(mode)
    try {
      const collected = await collectOrganizationRows<OrganizationTaskRow>(
        async (page, pageSize) => {
          const response = await listOrganizationTasks(
            surface,
            props.organizationId,
            { ...listParams, p: page, page_size: pageSize }
          )
          return (
            response.data ?? {
              page,
              page_size: pageSize,
              total: 0,
              items: [],
            }
          )
        }
      )
      await writeTaskExportFile(
        buildTaskExportSheet(
          collected.items.map(organizationTaskToLog),
          mode,
          t
        )
      )
      if (collected.truncated) {
        toast.warning(
          t('Exported the first {{exported}} of {{total}} records.', {
            exported: collected.items.length,
            total: collected.total,
          })
        )
      }
    } catch {
      toast.error(t('The export could not be prepared.'))
    } finally {
      setExporting(null)
    }
  }

  const { table } = useDataTable({
    data: rows,
    columns,
    columnFilters,
    onColumnFiltersChange,
    pagination,
    onPaginationChange,
    manualPagination: true,
    manualFiltering: true,
    totalCount: tasks.total,
    ensurePageInRange,
    columnVisibilityStorageKey: TASK_COLUMN_VISIBILITY_STORAGE_KEY,
  })

  const resetFilters = () => {
    onColumnFiltersChange([])
    navigate({
      search: (previous) => ({
        ...previous,
        startTime: undefined,
        endTime: undefined,
      }),
    })
    onPaginationChange((previous) => ({ ...previous, pageIndex: 0 }))
  }

  return (
    <DataTablePage
      table={table}
      columns={columns}
      isLoading={tasks.isLoading}
      isFetching={tasks.isFetching}
      emptyTitle={t('No tasks in this range')}
      emptyDescription={t(
        'Adjust the time range or the filters to find what the organization submitted.'
      )}
      skeletonKeyPrefix='organization-task-skeleton'
      applyHeaderSize
      toolbar={
        <LogsFilterToolbar
          table={table}
          hasActiveFilters={columnFilters.length > 0 || window.fromUrl}
          hasAdvancedActiveFilters={
            filters.status !== undefined || filters.action !== undefined
          }
          searchLoading={tasks.isFetching}
          onReset={resetFilters}
          onSearch={tasks.refetch}
          actionStart={
            <DropdownMenu modal={false}>
              <DropdownMenuTrigger
                render={
                  <Button
                    type='button'
                    variant='outline'
                    size='sm'
                    disabled={exporting !== null || tasks.total === 0}
                  />
                }
              >
                {exporting ? (
                  <Loader2 className='size-4 animate-spin' />
                ) : (
                  <Download className='size-4' />
                )}
                {exporting ? t('Exporting...') : t('Export')}
                <ChevronDown className='size-3.5' />
              </DropdownMenuTrigger>
              <DropdownMenuContent align='end'>
                <DropdownMenuItem onClick={() => void handleExport('detail')}>
                  {t('Excel (details)')}
                </DropdownMenuItem>
                <DropdownMenuItem onClick={() => void handleExport('daily')}>
                  {t('Excel (daily summary)')}
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          }
          primaryFilters={
            <>
              <LogsFilterField className='sm:col-span-2'>
                <CompactDateTimeRangePicker
                  start={window.start}
                  end={window.end}
                  onChange={({ start, end }) =>
                    navigate({
                      search: (previous) => ({
                        ...previous,
                        startTime: start?.getTime(),
                        endTime: end?.getTime(),
                      }),
                    })
                  }
                />
              </LogsFilterField>
              <OrganizationTextFilter
                value={filters.task_id}
                placeholder={t('Task ID')}
                onChange={(value) =>
                  setOrganizationTextFilter(
                    onColumnFiltersChange,
                    columnFilters,
                    'task_id',
                    value
                  )
                }
              />
            </>
          }
          advancedFilters={
            <>
              <LogsFilterField>
                <OrganizationSelectFilter
                  value={filters.status}
                  placeholder={t('Status')}
                  allLabel={t('All statuses')}
                  options={ORGANIZATION_TASK_STATUS_OPTIONS}
                  onChange={(value) =>
                    setOrganizationTextFilter(
                      onColumnFiltersChange,
                      columnFilters,
                      'status',
                      value
                    )
                  }
                />
              </LogsFilterField>
              <LogsFilterField>
                <OrganizationSelectFilter
                  value={filters.action}
                  placeholder={t('Type')}
                  allLabel={t('All Types')}
                  options={ORGANIZATION_TASK_ACTION_OPTIONS}
                  onChange={(value) =>
                    setOrganizationTextFilter(
                      onColumnFiltersChange,
                      columnFilters,
                      'action',
                      value
                    )
                  }
                />
              </LogsFilterField>
            </>
          }
        />
      }
    />
  )
}
