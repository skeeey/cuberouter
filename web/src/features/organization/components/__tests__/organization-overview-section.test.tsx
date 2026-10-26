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
import { render, screen, waitFor, within } from '@testing-library/react'
import { beforeAll, beforeEach, describe, expect, test, vi } from 'vitest'

import { getOrganizationQuotaData } from '../../api'
import type { Organization, OrganizationQuotaDataRow } from '../../types'
import { OrganizationOverviewSection } from '../organization-overview-section'

vi.mock('../organization-page-provider', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../organization-page-provider')>()),
  useOrganizationSurface: () => 'member',
}))

vi.mock('../../api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../api')>()),
  getOrganizationQuotaData: vi.fn(),
}))

const quotaDataMock = vi.mocked(getOrganizationQuotaData)

function organization(overrides: Partial<Organization> = {}): Organization {
  return {
    id: 7,
    name: 'Acme Research',
    slug: 'acme-research',
    status: 'active',
    group: 'default',
    quota: 5_000_000,
    used_quota: 2_000_000,
    request_count: 12,
    owner_user_id: 42,
    created_by: 42,
    created_at: 1_700_000_000,
    updated_at: 1_700_000_000,
    ...overrides,
  }
}

function quotaRow(
  overrides: Partial<OrganizationQuotaDataRow> = {}
): OrganizationQuotaDataRow {
  return {
    id: 1,
    user_id: 42,
    model_name: 'gpt-4o',
    created_at: 1_700_000_000,
    count: 10,
    quota: 500_000,
    ...overrides,
  }
}

function renderSection(
  props: Partial<Organization> = {},
  rows: OrganizationQuotaDataRow[] = [quotaRow()]
): void {
  quotaDataMock.mockResolvedValue({ success: true, data: rows })
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <OrganizationOverviewSection
        organization={organization(props)}
        canViewUsage
        onForbidden={() => undefined}
      />
    </QueryClientProvider>
  )
}

beforeEach(() => {
  quotaDataMock.mockReset()
})

// jsdom reports every element as 0x0, so Recharts would render an empty
// container and never draw the axis under test. Report a fixed size instead.
beforeAll(() => {
  class SizedResizeObserver {
    private readonly callback: ResizeObserverCallback

    constructor(callback: ResizeObserverCallback) {
      this.callback = callback
    }

    observe(): void {
      this.callback(
        [{ contentRect: { width: 320, height: 200 } } as ResizeObserverEntry],
        this as unknown as ResizeObserver
      )
    }

    unobserve(): void {}
    disconnect(): void {}
  }

  Object.defineProperty(globalThis, 'ResizeObserver', {
    configurable: true,
    value: SizedResizeObserver,
  })
})

describe('organization overview summary', () => {
  test('shows the total grant and the remaining balance derived from it', async () => {
    renderSection({ quota: 5_000_000, used_quota: 2_000_000 })

    expect(screen.getByText('Total Quota')).toBeInTheDocument()
    await waitFor(
      () => {
        expect(screen.getByText('$10')).toBeInTheDocument()
        expect(screen.getByText('Remaining $6')).toBeInTheDocument()
      },
      { timeout: 5_000 }
    )
  })

  test('never shows a negative remaining balance when usage exceeds the grant', async () => {
    renderSection({ quota: 1_000_000, used_quota: 2_000_000 })

    await waitFor(
      () => {
        expect(screen.getByText('Remaining $0')).toBeInTheDocument()
      },
      { timeout: 5_000 }
    )
  })
})

describe('organization overview chart range', () => {
  test('places the date range below the summary cards and above the charts', () => {
    renderSection()

    const totalQuota = screen.getByText('Total Quota')
    const rangeButton = screen.getByRole('button', {
      name: /^\d{4}-\d{2}-\d{2} ~ \d{4}-\d{2}-\d{2}$/,
    })
    const requestsChart = screen.getByText('Requests by Model')

    expect(
      totalQuota.compareDocumentPosition(rangeButton) &
        Node.DOCUMENT_POSITION_FOLLOWING
    ).not.toBe(0)
    expect(
      rangeButton.compareDocumentPosition(requestsChart) &
        Node.DOCUMENT_POSITION_FOLLOWING
    ).not.toBe(0)
  })

  test('formats the quota chart in currency units', async () => {
    renderSection()

    const chartTitle = await screen.findByText('Quota by Model')
    const chartCard = chartTitle.closest<HTMLElement>('[data-slot="card"]')
    expect(chartCard).not.toBeNull()

    expect(
      (await within(chartCard as HTMLElement).findAllByText(/\$/)).length
    ).toBeGreaterThan(0)
  })
})
