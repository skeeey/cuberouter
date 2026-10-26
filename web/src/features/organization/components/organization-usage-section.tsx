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
import { CompactDateTimeRangePicker } from '@/features/usage-logs/components/compact-date-time-range-picker'
import {
  LogsFilterField,
  LogsFilterToolbar,
} from '@/features/usage-logs/components/logs-filter-toolbar'
import { parseLogOther } from '@/features/usage-logs/lib/format'
import { useTableUrlState } from '@/hooks/use-table-url-state'
import dayjs from '@/lib/dayjs'
import { formatLogQuota, formatNumber, formatTimestamp } from '@/lib/format'

import { listOrganizationLogs } from '../api'
import { useOrganizationPagedSection } from '../hooks/use-organization-paged-query'
import {
  collectOrganizationRows,
  organizationColumnFilterValue,
  organizationLogBillingMode,
  organizationLogModelInfo,
  organizationLogOriginalQuota,
  organizationLogResponsibleName,
  organizationLogTimeRangeParams,
  organizationLogTimeWindow,
  organizationLogTokenName,
  ORGANIZATION_LOG_BILLING_MODE_LABELS,
  setOrganizationTextFilter,
  type OrganizationLogBillingMode,
} from '../lib'
import { ORGANIZATION_DEFAULT_PAGE_SIZE } from '../lib/organization-pagination'
import {
  buildUsageExportSheet,
  writeUsageExportFile,
  type UsageExportMode,
} from '../lib/organization-usage-export'
import type { OrganizationLogRow } from '../types'
import { OrganizationTextFilter } from './organization-filter-fields'
import {
  useOrganizationSectionRoute,
  useOrganizationSurface,
} from './organization-page-provider'
import {
  OrganizationSection,
  OrganizationSectionEmpty,
} from './organization-section'

type OrganizationUsageSectionProps = {
  organizationId: number
  /** The caller may read the organization's usage. */
  canView: boolean
  /** The caller reads every member's rows, not just their own. */
  canViewWideData: boolean
  /** Leaves the page when the backend refuses the read. */
  onForbidden: () => void
}

/**
 * The organization's consumption, one row per request.
 *
 * The table is built from the request log rather than the billing ledger: the
 * ledger records balance movements, while this page answers what a request
 * cost, which model and upstream served it, and how it was priced. Only
 * consumption rows are read; top-ups, refunds and errors are not usage.
 */
export function OrganizationUsageSection(props: OrganizationUsageSectionProps) {
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
      { columnId: 'model_name', searchKey: 'usageModel', type: 'string' },
      { columnId: 'token_name', searchKey: 'usageToken', type: 'string' },
      {
        columnId: 'responsible_user_id',
        searchKey: 'usageResponsible',
        type: 'string',
      },
    ],
  })

  const window = organizationLogTimeWindow(search, defaultUsageWindow())
  const timeRange = organizationLogTimeRangeParams(window.start, window.end)

  const filters = {
    model_name: organizationColumnFilterValue(columnFilters, 'model_name'),
    token_name: organizationColumnFilterValue(columnFilters, 'token_name'),
    responsible_name: props.canViewWideData
      ? organizationColumnFilterValue(columnFilters, 'responsible_user_id')
      : undefined,
  }

  const listParams = {
    p: pagination.pageIndex + 1,
    page_size: pagination.pageSize,
    type: 2,
    ...filters,
    ...timeRange,
  }

  const records = useOrganizationPagedSection<OrganizationLogRow>({
    surface,
    organizationId: props.organizationId,
    resource: 'usage',
    params: listParams,
    query: () =>
      listOrganizationLogs(surface, props.organizationId, listParams),
    enabled: props.canView,
    onForbidden: props.onForbidden,
  })

  const [exporting, setExporting] = useState<UsageExportMode | null>(null)

  const handleExport = async (mode: UsageExportMode) => {
    setExporting(mode)
    try {
      const collected = await collectOrganizationRows<OrganizationLogRow>(
        async (page, pageSize) => {
          const response = await listOrganizationLogs(
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
      await writeUsageExportFile(
        buildUsageExportSheet(collected.items, mode, t)
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

  const columns = buildUsageColumns(t)

  const { table } = useDataTable({
    data: records.items,
    columns,
    columnFilters,
    onColumnFiltersChange,
    pagination,
    onPaginationChange,
    manualPagination: true,
    manualFiltering: true,
    totalCount: records.total,
    ensurePageInRange,
    columnVisibilityStorageKey: 'organization-usage-column-visibility',
  })

  if (!props.canView) {
    return (
      <OrganizationSectionEmpty
        icon='usage'
        title={t('Usage')}
        message={t(
          'Only an organization owner or administrator can see organization-wide usage.'
        )}
      />
    )
  }

  const isFiltered = Object.values(filters).some(
    (value) => value !== undefined && value !== ''
  )

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
    <OrganizationSection
      icon='usage'
      title={t('Usage')}
      description={t('Per-request usage and charges for the organization.')}
      count={records.total}
    >
      <DataTablePage
        table={table}
        columns={columns}
        isLoading={records.isLoading}
        isFetching={records.isFetching}
        emptyTitle={t('No usage in this period.')}
        emptyDescription={t('Adjust the time range or filters to find usage.')}
        skeletonKeyPrefix='organization-usage-skeleton'
        applyHeaderSize
        toolbar={
          <LogsFilterToolbar
            table={table}
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
                  value={filters.model_name}
                  placeholder={t('Model')}
                  onChange={(value) =>
                    setOrganizationTextFilter(
                      onColumnFiltersChange,
                      columnFilters,
                      'model_name',
                      value
                    )
                  }
                />
                <OrganizationTextFilter
                  value={filters.token_name}
                  placeholder={t('API Key Name')}
                  onChange={(value) =>
                    setOrganizationTextFilter(
                      onColumnFiltersChange,
                      columnFilters,
                      'token_name',
                      value
                    )
                  }
                />
                {props.canViewWideData ? (
                  <OrganizationTextFilter
                    value={filters.responsible_name}
                    placeholder={t('User')}
                    onChange={(value) =>
                      setOrganizationTextFilter(
                        onColumnFiltersChange,
                        columnFilters,
                        'responsible_user_id',
                        value
                      )
                    }
                  />
                ) : null}
              </>
            }
            hasActiveFilters={isFiltered || window.fromUrl}
            searchLoading={records.isFetching}
            onReset={resetFilters}
            onSearch={records.refetch}
            actionStart={
              <DropdownMenu modal={false}>
                <DropdownMenuTrigger
                  render={
                    <Button
                      type='button'
                      variant='outline'
                      size='sm'
                      disabled={exporting !== null || records.total === 0}
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
          />
        }
      />
    </OrganizationSection>
  )
}

/** The month-to-date window the page opens on. */
function defaultUsageWindow(): { start: Date; end: Date } {
  return {
    start: dayjs().startOf('month').toDate(),
    end: dayjs().add(1, 'hour').toDate(),
  }
}

function buildUsageColumns(
  t: (key: string) => string
): ColumnDef<OrganizationLogRow>[] {
  return [
    {
      id: 'responsible_user_id',
      accessorFn: (row) => organizationLogResponsibleName(row),
      header: () => <span className='whitespace-nowrap'>{t('User')}</span>,
      size: 150,
      cell: ({ getValue }) => (
        <span className='font-medium whitespace-nowrap'>
          {String(getValue())}
        </span>
      ),
    },
    {
      id: 'token_name',
      accessorFn: (row) => organizationLogTokenName(row),
      header: () => (
        <span className='whitespace-nowrap'>{t('API Key Name')}</span>
      ),
      size: 160,
      cell: ({ getValue }) => (
        <span className='whitespace-nowrap'>{String(getValue())}</span>
      ),
    },
    {
      id: 'model_name',
      accessorKey: 'model_name',
      header: () => <span className='whitespace-nowrap'>{t('Model')}</span>,
      size: 180,
      cell: ({ row }) => (
        <span className='block max-w-44 truncate font-mono text-xs'>
          {row.original.model_name || '-'}
        </span>
      ),
    },
    {
      id: 'upstream_model',
      accessorFn: (row) => organizationLogModelInfo(row).actualModel || '',
      header: () => <span className='whitespace-nowrap'>{t('Upstream')}</span>,
      size: 180,
      cell: ({ getValue }) => {
        const upstream = String(getValue())
        return (
          <span className='block max-w-44 truncate font-mono text-xs'>
            {upstream || '-'}
          </span>
        )
      },
    },
    {
      id: 'billing_mode',
      accessorFn: (row) =>
        organizationLogBillingMode(parseLogOther(row.other ?? '')),
      header: () => (
        <span className='whitespace-nowrap'>{t('Billing Mode')}</span>
      ),
      size: 120,
      cell: ({ getValue }) => {
        const mode = getValue() as OrganizationLogBillingMode | undefined
        return (
          <span className='whitespace-nowrap'>
            {mode ? t(ORGANIZATION_LOG_BILLING_MODE_LABELS[mode]) : '-'}
          </span>
        )
      },
    },
    {
      id: 'prompt_tokens',
      accessorKey: 'prompt_tokens',
      header: () => (
        <span className='whitespace-nowrap'>{t('Prompt Tokens')}</span>
      ),
      size: 110,
      cell: ({ row }) => (
        <span className='tabular-nums'>
          {formatNumber(row.original.prompt_tokens || 0)}
        </span>
      ),
    },
    {
      id: 'completion_tokens',
      accessorKey: 'completion_tokens',
      header: () => (
        <span className='whitespace-nowrap'>{t('Completion Tokens')}</span>
      ),
      size: 110,
      cell: ({ row }) => (
        <span className='tabular-nums'>
          {formatNumber(row.original.completion_tokens || 0)}
        </span>
      ),
    },
    {
      id: 'cache_tokens',
      accessorFn: (row) => parseLogOther(row.other ?? '')?.cache_tokens || 0,
      header: () => (
        <span className='whitespace-nowrap'>{t('Cache Read Tokens')}</span>
      ),
      size: 130,
      cell: ({ getValue }) => (
        <span className='tabular-nums'>
          {formatNumber(Number(getValue()) || 0)}
        </span>
      ),
    },
    {
      id: 'original_quota',
      accessorFn: (row) => organizationLogOriginalQuota(row),
      header: () => (
        <span className='whitespace-nowrap'>{t('Original price')}</span>
      ),
      size: 130,
      cell: ({ getValue }) => (
        <span className='tabular-nums'>
          {formatLogQuota(Number(getValue()) || 0)}
        </span>
      ),
    },
    {
      id: 'quota',
      accessorKey: 'quota',
      header: () => (
        <span className='whitespace-nowrap'>{t('User charge')}</span>
      ),
      size: 130,
      cell: ({ row }) => (
        <span className='tabular-nums'>
          {formatLogQuota(row.original.quota || 0)}
        </span>
      ),
    },
    {
      id: 'created_at',
      accessorKey: 'created_at',
      header: () => <span className='whitespace-nowrap'>{t('Time')}</span>,
      size: 180,
      cell: ({ row }) => (
        <span className='whitespace-nowrap'>
          {formatTimestamp(row.original.created_at)}
        </span>
      ),
    },
  ]
}
