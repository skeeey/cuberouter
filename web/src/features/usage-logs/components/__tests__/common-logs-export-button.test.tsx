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
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, test, vi } from 'vitest'

import type { UsageLog } from '../../data/schema'
import { fetchLogsByCategory } from '../../lib/utils'
import { CommonLogsExportButton } from '../common-logs-export-button'

vi.mock('../../lib/utils', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/utils')>()),
  fetchLogsByCategory: vi.fn(),
}))

const exportMocks = vi.hoisted(() => ({
  writeCommonLogsExportFile: vi.fn(),
}))
vi.mock('../../lib/logs-export', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/logs-export')>()),
  writeCommonLogsExportFile: exportMocks.writeCommonLogsExportFile,
}))

const toastMock = vi.hoisted(() => ({
  warning: vi.fn(),
  error: vi.fn(),
}))
vi.mock('sonner', () => ({ toast: toastMock }))

const fetchLogsMock = vi.mocked(fetchLogsByCategory)

function row(): UsageLog {
  return {
    id: 1,
    user_id: 42,
    created_at: 1_700_000_000,
    type: 2,
    content: '',
    username: 'alice',
    token_name: 'key-1',
    model_name: 'deepseek-flash',
    quota: 800_000,
    prompt_tokens: 10,
    completion_tokens: 5,
    use_time: 1,
    is_stream: false,
    channel: 3,
    channel_name: 'prod',
    token_id: 9,
    group: 'default',
    ip: '',
    other: JSON.stringify({ request_path: '/v1/chat/completions' }),
    request_id: 'req-1',
    upstream_request_id: '',
  }
}

function renderButton(total = 1): void {
  render(
    <CommonLogsExportButton
      isAdmin={false}
      total={total}
      searchParams={{ model: 'deepseek-flash' }}
      columnFilters={[]}
    />
  )
}

beforeEach(() => {
  fetchLogsMock.mockReset()
  exportMocks.writeCommonLogsExportFile.mockReset()
  toastMock.warning.mockReset()
  toastMock.error.mockReset()
})

describe('common logs export button', () => {
  test('exports the filtered set through the page API and writes one workbook', async () => {
    fetchLogsMock.mockResolvedValue({
      success: true,
      data: { page: 1, page_size: 100, total: 1, items: [row()] },
    })
    exportMocks.writeCommonLogsExportFile.mockResolvedValue(undefined)

    renderButton()
    await userEvent.click(screen.getByRole('button', { name: 'Export' }))

    await waitFor(() =>
      expect(exportMocks.writeCommonLogsExportFile).toHaveBeenCalledTimes(1)
    )
    expect(fetchLogsMock).toHaveBeenCalledWith(
      expect.objectContaining({
        logCategory: 'common',
        isAdmin: false,
        page: 1,
        pageSize: 100,
        columnFilters: [],
      })
    )
    const searchParams = fetchLogsMock.mock.calls[0][0].searchParams as Record<
      string,
      unknown
    >
    expect(searchParams.model).toBe('deepseek-flash')
    expect(searchParams.startTime).toEqual(expect.any(Number))
    expect(searchParams.endTime).toEqual(expect.any(Number))
    expect(searchParams.endTime as number).toBeLessThanOrEqual(Date.now())
    expect(searchParams.startTime as number).toBeLessThanOrEqual(
      searchParams.endTime as number
    )
    const sheet = exportMocks.writeCommonLogsExportFile.mock.calls[0][0]
    expect(sheet.filename).toMatch(/^usage-logs-\d{4}-\d{2}-\d{2}-\d{6}\.xlsx$/)
    expect(sheet.rows).toHaveLength(2)
    expect(toastMock.warning).not.toHaveBeenCalled()
    expect(toastMock.error).not.toHaveBeenCalled()
  })

  test('pins one time window across every page', async () => {
    fetchLogsMock.mockImplementation(async (config) => {
      if (config.page === 1) {
        return {
          success: true,
          data: {
            page: 1,
            page_size: 100,
            total: 150,
            items: Array.from({ length: 100 }, row),
          },
        }
      }
      return {
        success: true,
        data: {
          page: 2,
          page_size: 100,
          total: 150,
          items: Array.from({ length: 50 }, row),
        },
      }
    })
    exportMocks.writeCommonLogsExportFile.mockResolvedValue(undefined)

    renderButton()
    await userEvent.click(screen.getByRole('button', { name: 'Export' }))

    await waitFor(() =>
      expect(exportMocks.writeCommonLogsExportFile).toHaveBeenCalledTimes(1)
    )
    expect(fetchLogsMock).toHaveBeenCalledTimes(2)
    const first = fetchLogsMock.mock.calls[0][0].searchParams
    const second = fetchLogsMock.mock.calls[1][0].searchParams
    expect(second.startTime).toBe(first.startTime)
    expect(second.endTime).toBe(first.endTime)
  })

  test('reports a failed export without writing a file', async () => {
    fetchLogsMock.mockResolvedValue({
      success: false,
      message: 'boom',
    })

    renderButton()
    await userEvent.click(screen.getByRole('button', { name: 'Export' }))

    await waitFor(() =>
      expect(toastMock.error).toHaveBeenCalledWith(
        'The export could not be prepared.'
      )
    )
    expect(exportMocks.writeCommonLogsExportFile).not.toHaveBeenCalled()
  })

  test('is disabled when the filtered set is empty', () => {
    renderButton(0)

    expect(screen.getByRole('button', { name: 'Export' })).toBeDisabled()
  })
})
