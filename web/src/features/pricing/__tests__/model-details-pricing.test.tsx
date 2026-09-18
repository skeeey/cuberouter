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
import { formatGroupPrice } from '../lib/price'
import type { PricingModel } from '../types'

// @lobehub/icons transitively imports @emoji-mart JSON assets that vitest's
// externalized ESM loader rejects. Icon rendering is irrelevant to the
// pricing/group contracts under test, so the icon loader boundary is stubbed.
vi.mock('@/lib/lobe-icon', () => ({
  getLobeIcon: () => null,
}))

// @visactor/vchart ships ESM that vitest's externalized loader cannot resolve.
// Charts live on the performance tab, which is never opened here, so the chart
// boundary is stubbed.
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
    enable_groups: ['vip'],
    ...overrides,
  }
}

function renderDetails(
  model: PricingModel,
  options?: {
    groupRatio?: Record<string, number>
    usableGroup?: Record<string, { desc: string; ratio: number }>
  }
) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <ModelDetailsContent
        model={model}
        groupRatio={options?.groupRatio ?? {}}
        usableGroup={options?.usableGroup ?? {}}
        endpointMap={{}}
        autoGroups={[]}
        priceRate={1}
        usdExchangeRate={1}
        tokenUnit='M'
      />
    </QueryClientProvider>
  )
}

describe('ModelDetailsContent pricing by viewer group', () => {
  test('anonymous viewers see the best (minimum) group price', () => {
    const model = pricingModel({ enable_groups: ['cheap', 'vip'] })
    renderDetails(model, {
      groupRatio: { cheap: 0.5, vip: 2 },
      usableGroup: {},
    })

    const expected = formatGroupPrice(model, 'cheap', 'input', 'M', false, 1, 1, {
      cheap: 0.5,
      vip: 2,
    })
    expect(screen.getByText(expected)).toBeInTheDocument()
  })

  test('anonymous fallback ignores model.group_ratio', () => {
    const model = pricingModel({
      enable_groups: ['cheap', 'vip'],
      group_ratio: { cheap: 9, vip: 9 },
    })
    renderDetails(model, {
      groupRatio: { cheap: 0.5, vip: 2 },
      usableGroup: {},
    })

    const expected = formatGroupPrice(model, 'cheap', 'input', 'M', false, 1, 1, {
      cheap: 0.5,
      vip: 2,
    })
    expect(screen.getByText(expected)).toBeInTheDocument()
    const wrong = formatGroupPrice(model, 'cheap', 'input', 'M', false, 1, 1, {
      cheap: 9,
      vip: 9,
    })
    expect(screen.queryByText(wrong)).not.toBeInTheDocument()
  })

  test('viewer with several usable groups gets the minimum ratio regardless of key order', () => {
    const model = pricingModel({ enable_groups: ['zzz', 'aaa'] })
    renderDetails(model, {
      groupRatio: { zzz: 0.5, aaa: 2 },
      usableGroup: {
        zzz: { desc: '', ratio: 0.5 },
        aaa: { desc: '', ratio: 2 },
      },
    })

    const expected = formatGroupPrice(model, 'zzz', 'input', 'M', false, 1, 1, {
      zzz: 0.5,
      aaa: 2,
    })
    expect(screen.getByText(expected)).toBeInTheDocument()
  })

  test('a logged-in viewer locked to one group sees that group price', () => {
    const model = pricingModel({ enable_groups: ['cheap', 'vip'] })
    renderDetails(model, {
      groupRatio: { vip: 2 },
      usableGroup: { vip: { desc: '', ratio: 2 } },
    })

    const expected = formatGroupPrice(model, 'vip', 'input', 'M', false, 1, 1, {
      vip: 2,
    })
    expect(screen.getByText(expected)).toBeInTheDocument()
    const anonymous = formatGroupPrice(
      model,
      'cheap',
      'input',
      'M',
      false,
      1,
      1,
      { cheap: 0.5 }
    )
    expect(screen.queryByText(anonymous)).not.toBeInTheDocument()
  })

  test('per-request models scale the fixed price by the viewer group ratio', () => {
    const model = pricingModel({
      quota_type: 1,
      model_price: 0.1,
      enable_groups: ['vip'],
    })
    renderDetails(model, {
      groupRatio: { vip: 2 },
      usableGroup: { vip: { desc: '', ratio: 2 } },
    })

    // 0.1 USD × 2 → $0.2 per request
    expect(screen.getByText('$0.2')).toBeInTheDocument()
    expect(screen.queryByText('$0.1')).not.toBeInTheDocument()
  })
})

describe('ModelDetailsContent overview layout', () => {
  test('moves the billing mode badge out of the header into the pricing section', () => {
    renderDetails(pricingModel({}), {
      groupRatio: { vip: 1 },
      usableGroup: { vip: { desc: '', ratio: 1 } },
    })

    // Header: only the model name + copy button, no badge
    const header = document.querySelector('header')
    expect(header).not.toBeNull()
    expect(header).not.toHaveTextContent('Token-based')
    // The pricing section still shows the billing mode
    expect(screen.getByText('Token-based')).toBeInTheDocument()
  })

  test('drops the Base Price heading and the group pricing table', () => {
    renderDetails(pricingModel({}), {
      groupRatio: { vip: 1 },
      usableGroup: { vip: { desc: '', ratio: 1 } },
    })

    expect(screen.queryByText('Base Price')).not.toBeInTheDocument()
    expect(screen.queryByText('Pricing by Group')).not.toBeInTheDocument()
    expect(screen.queryByText('Auto Group Chain')).not.toBeInTheDocument()
    // The pricing section itself stays
    expect(screen.getByText('Pricing')).toBeInTheDocument()
  })

  test('provider section keeps provider/endpoints/tags and drops type/groups/parameters', () => {
    renderDetails(
      pricingModel({
        vendor_name: 'Test Vendor',
        supported_endpoint_types: ['openai'],
        tags: 'hot,new',
        parameter_count: '7B',
        enable_groups: ['vip'],
      }),
      {
        groupRatio: { vip: 1 },
        usableGroup: { vip: { desc: '', ratio: 1 } },
      }
    )

    expect(screen.getByText('Provider')).toBeInTheDocument()
    expect(screen.getByText('Endpoints')).toBeInTheDocument()
    expect(screen.getByText('Tags')).toBeInTheDocument()
    expect(screen.queryByText('Type')).not.toBeInTheDocument()
    expect(screen.queryByText('Groups')).not.toBeInTheDocument()
    expect(screen.queryByText('Parameters')).not.toBeInTheDocument()
  })
})
