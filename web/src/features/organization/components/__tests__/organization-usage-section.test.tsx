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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, test, vi } from 'vitest'

import { formatLogQuota } from '@/lib/format'

import { listOrganizationLogs } from '../../api'
import type { OrganizationLogRow } from '../../types'
import { OrganizationUsageSection } from '../organization-usage-section'

// The URL is the page's state, not this section's; the harness supplies a fixed
// empty set of search params instead of standing up a route.
const pageRoute = vi.hoisted(() => ({ search: {}, navigate: vi.fn() }))

vi.mock('../organization-page-provider', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../organization-page-provider')>()),
  useOrganizationSectionRoute: () => pageRoute,
  useOrganizationSurface: () => 'member',
}))

vi.mock('../../api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../api')>()),
  listOrganizationLogs: vi.fn(),
}))

const exportMock = vi.hoisted(() => ({
  buildUsageExportSheet: vi.fn(() => ({
    rows: [],
    columns: [],
    filename: 'organization-usage.xlsx',
  })),
  writeUsageExportFile: vi.fn(),
}))

vi.mock('../../lib/organization-usage-export', () => exportMock)

const listLogsMock = vi.mocked(listOrganizationLogs)

const ORGANIZATION_ID = 7

function usageRow(
  overrides: Partial<OrganizationLogRow> = {}
): OrganizationLogRow {
  return {
    id: 1,
    user_id: 42,
    created_at: 1_700_000_000,
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

function renderSection(): void {
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <OrganizationUsageSection
        organizationId={ORGANIZATION_ID}
        canView
        canViewWideData
        onForbidden={() => undefined}
      />
    </QueryClientProvider>
  )
}

beforeEach(() => {
  listLogsMock.mockReset().mockResolvedValue({
    success: true,
    data: {
      page: 1,
      page_size: 20,
      total: 1,
      items: [usageRow()],
    },
  })
})

describe('organization usage section', () => {
  test('renders one request-level usage table without the old overview cards', () => {
    renderSection()

    for (const header of [
      'User',
      'API Key Name',
      'Model',
      'Upstream',
      'Billing Mode',
      'Prompt Tokens',
      'Completion Tokens',
      'Cache Read Tokens',
      'Original price',
      'User charge',
      'Time',
    ]) {
      expect(
        screen.getByRole('columnheader', { name: header })
      ).toBeInTheDocument()
    }

    expect(screen.queryByText('Total cost')).not.toBeInTheDocument()
    expect(screen.queryByText('Active members')).not.toBeInTheDocument()
    expect(screen.queryByText('Top spender')).not.toBeInTheDocument()
    expect(screen.queryByRole('tab')).not.toBeInTheDocument()
  })

  test('shows the upstream model, billing mode, original price and charged quota', async () => {
    renderSection()

    expect(
      await screen.findByText('Alice', undefined, { timeout: 5_000 })
    ).toBeInTheDocument()
    expect(screen.getByText('key-1')).toBeInTheDocument()
    expect(screen.getByText('gpt-4o')).toBeInTheDocument()
    expect(screen.getByText('gpt-4o-upstream')).toBeInTheDocument()
    expect(screen.getByText('Ratio')).toBeInTheDocument()
    expect(screen.getByText('300')).toBeInTheDocument()
    expect(screen.getByText(formatLogQuota(1_000_000))).toBeInTheDocument()
    expect(screen.getByText(formatLogQuota(800_000))).toBeInTheDocument()
  })

  test('reads only consumption logs for the selected window', async () => {
    renderSection()

    await waitFor(
      () => {
        expect(listLogsMock).toHaveBeenCalled()
      },
      { timeout: 5_000 }
    )

    const params = listLogsMock.mock.calls[0]?.[2]
    expect(params?.type).toBe(2)
    expect(params).toHaveProperty('start_timestamp')
    expect(params).toHaveProperty('end_timestamp')
    expect(params).not.toHaveProperty('group')
  })

  test('exports the whole filtered set in detail or daily mode', async () => {
    renderSection()

    await screen.findByText('Alice', undefined, { timeout: 5_000 })

    await userEvent.click(screen.getByRole('button', { name: /Export/ }))
    await userEvent.click(
      await screen.findByRole('menuitem', { name: 'Excel (details)' })
    )

    await waitFor(() => {
      expect(exportMock.writeUsageExportFile).toHaveBeenCalled()
    })
    expect(exportMock.buildUsageExportSheet).toHaveBeenLastCalledWith(
      expect.arrayContaining([expect.objectContaining({ id: 1 })]),
      'detail',
      expect.any(Function)
    )
    expect(
      listLogsMock.mock.calls.some(([, , params]) => params.page_size === 100)
    ).toBe(true)

    await userEvent.click(screen.getByRole('button', { name: /Export/ }))
    await userEvent.click(
      await screen.findByRole('menuitem', { name: 'Excel (daily summary)' })
    )

    await waitFor(() => {
      expect(exportMock.buildUsageExportSheet).toHaveBeenLastCalledWith(
        expect.any(Array),
        'daily',
        expect.any(Function)
      )
    })
  })
})
