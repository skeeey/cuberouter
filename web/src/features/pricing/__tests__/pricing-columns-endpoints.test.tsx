/*
Copyright (C) 2023-2026 QuantumNous

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
import {
  flexRender,
  getCoreRowModel,
  useReactTable,
} from '@tanstack/react-table'
import { render, screen } from '@testing-library/react'
import { describe, expect, test, vi } from 'vitest'

import { usePricingColumns } from '../components/pricing-columns'
import type { PricingModel } from '../types'

// @lobehub/icons transitively imports @emoji-mart JSON assets that vitest's
// externalized ESM loader rejects. Icon rendering is irrelevant to the endpoint
// labels under test, so the icon loader boundary is stubbed.
vi.mock('@/lib/lobe-icon', () => ({
  getLobeIcon: () => null,
}))

function pricingModel(overrides: Partial<PricingModel>): PricingModel {
  return {
    id: 1,
    model_name: 'test-model',
    quota_type: 0,
    model_ratio: 1,
    completion_ratio: 2,
    enable_groups: ['default'],
    ...overrides,
  }
}

/**
 * Renders just the endpoints cell through the real table API, so the assertion
 * covers the column definition the pricing table mounts rather than a copy of
 * its rendering logic.
 */
function EndpointsCell(props: { model: PricingModel }) {
  const columns = usePricingColumns()
  const table = useReactTable({
    data: [props.model],
    columns,
    getCoreRowModel: getCoreRowModel(),
  })
  const cell = table
    .getRowModel()
    .rows[0].getVisibleCells()
    .find((candidate) => candidate.column.id === 'supported_endpoint_types')

  if (!cell) return null

  return <>{flexRender(cell.column.columnDef.cell, cell.getContext())}</>
}

describe('pricing table endpoints column', () => {
  test('labels every endpoint the way the rest of the catalog does', () => {
    render(
      <EndpointsCell
        model={pricingModel({
          supported_endpoint_types: ['openai', 'openai-response'],
        })}
      />
    )

    expect(screen.getByText('Chat')).toBeInTheDocument()
    expect(screen.getByText('Response')).toBeInTheDocument()
    expect(screen.queryByText('openai')).not.toBeInTheDocument()
  })

  test('reads a per-second video model as the single video endpoint', () => {
    render(
      <EndpointsCell
        model={pricingModel({
          supported_endpoint_types: ['openai'],
          video_prices: {
            rows: [
              { resolution: '720p', normal_price: 0.1, off_peak_price: 0.07 },
            ],
          },
        })}
      />
    )

    expect(screen.getByText('Video')).toBeInTheDocument()
    expect(screen.queryByText('Chat')).not.toBeInTheDocument()
  })
})
