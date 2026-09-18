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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { describe, expect, test, vi } from 'vitest'

import { ModelDetailsContent } from '../components/model-details'
import type { PricingModel } from '../types'

// @lobehub/icons transitively imports @emoji-mart JSON assets that vitest's
// externalized ESM loader rejects. Icon rendering is irrelevant to the endpoint
// labels under test, so the icon loader boundary is stubbed.
vi.mock('@/lib/lobe-icon', () => ({
  getLobeIcon: () => null,
}))

// @visactor/vchart ships ESM that vitest's externalized loader cannot resolve.
// Charts live on the performance tab, which the endpoint-label contracts under
// test never open, so the chart boundary is stubbed.
vi.mock('@visactor/react-vchart', () => ({
  VChart: () => null,
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

function renderDetails(model: PricingModel) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <ModelDetailsContent
        model={model}
        groupRatio={{}}
        usableGroup={{}}
        endpointMap={{}}
        autoGroups={[]}
        priceRate={1}
        usdExchangeRate={1}
        tokenUnit='M'
      />
    </QueryClientProvider>
  )
}

function endpointsCell() {
  return screen.getByText('Endpoints').parentElement
}

describe('ModelDetailsContent endpoints', () => {
  test('lists the endpoints under their display labels', () => {
    renderDetails(
      pricingModel({
        supported_endpoint_types: ['openai', 'openai-response', 'anthropic'],
      })
    )

    const cell = endpointsCell()
    expect(cell).toHaveTextContent('Chat')
    expect(cell).toHaveTextContent('Response')
    expect(cell).toHaveTextContent('Anthropic')
    expect(cell).not.toHaveTextContent('openai')
  })

  test('reads a per-second video model as the single video endpoint', () => {
    renderDetails(
      pricingModel({
        supported_endpoint_types: ['openai'],
        video_prices: {
          rows: [
            { resolution: '720p', normal_price: 0.1, off_peak_price: 0.07 },
          ],
        },
      })
    )

    const cell = endpointsCell()
    expect(cell).toHaveTextContent('Video')
    expect(cell).not.toHaveTextContent('Chat')
  })
})
