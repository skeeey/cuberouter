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
import type { ColumnFiltersState } from '@tanstack/react-table'
import { Download, Loader2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'

import {
  buildCommonLogsExportSheet,
  collectCommonLogRows,
  pinLogsExportTimeRange,
  writeCommonLogsExportFile,
  type CommonLogsExportPage,
} from '../lib/logs-export'
import { fetchLogsByCategory } from '../lib/utils'

interface CommonLogsExportButtonProps {
  isAdmin: boolean
  /** The filtered set's size; an empty set has nothing to export. */
  total: number
  searchParams: Record<string, unknown>
  columnFilters: ColumnFiltersState
}

/**
 * Exports every log matching the table's applied filters, not just the current
 * page. The page API clamps `page_size` to 100, so the rows are assembled one
 * page at a time up to the export cap.
 */
export function CommonLogsExportButton(props: CommonLogsExportButtonProps) {
  const { t } = useTranslation()
  const [exporting, setExporting] = useState(false)

  const handleExport = async () => {
    setExporting(true)
    try {
      const exportSearchParams = pinLogsExportTimeRange(props.searchParams)
      const collected = await collectCommonLogRows(async (page, pageSize) => {
        const result = await fetchLogsByCategory({
          logCategory: 'common',
          isAdmin: props.isAdmin,
          page,
          pageSize,
          searchParams: exportSearchParams,
          columnFilters: props.columnFilters,
        })
        if (!result?.success) {
          throw new Error(result?.message || 'export failed')
        }
        // logCategory is 'common', so the union response holds UsageLog rows.
        return result.data as CommonLogsExportPage | undefined
      })
      await writeCommonLogsExportFile(
        buildCommonLogsExportSheet(collected.items, props.isAdmin, t)
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
      setExporting(false)
    }
  }

  return (
    <Button
      type='button'
      variant='outline'
      size='sm'
      disabled={exporting || props.total === 0}
      onClick={() => void handleExport()}
    >
      {exporting ? (
        <Loader2 className='size-4 animate-spin' />
      ) : (
        <Download className='size-4' />
      )}
      {exporting ? t('Exporting...') : t('Export')}
    </Button>
  )
}
