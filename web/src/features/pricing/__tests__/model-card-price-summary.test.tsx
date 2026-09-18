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
import { render, screen } from '@testing-library/react'
import { describe, expect, test, vi } from 'vitest'

import { ModelCard } from '../components/model-card'
import type { PricingModel } from '../types'

// @lobehub/icons transitively imports @emoji-mart JSON assets that vitest's
// externalized ESM loader rejects. Icon rendering is irrelevant to the price
// summary contracts under test, so the icon loader boundary is stubbed.
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

function renderCard(model: PricingModel) {
  render(<ModelCard model={model} onClick={() => undefined} />)
}

describe('ModelCard price summary', () => {
  test('token model with a cache ratio shows only input and output prices', () => {
    renderCard(pricingModel({ cache_ratio: 0.5 }))

    // input = 1 * 2 * 1 = $2, output = $2 * 2 = $4 per 1M tokens
    expect(screen.getByText('Input')).toBeInTheDocument()
    expect(screen.getByText('$2')).toBeInTheDocument()
    expect(screen.getByText('Output')).toBeInTheDocument()
    expect(screen.getByText('$4')).toBeInTheDocument()
    expect(screen.queryByText('Cached')).not.toBeInTheDocument()
    expect(screen.queryByText('$1')).not.toBeInTheDocument()
  })

  test('video model shows a dash input and the lowest per-second price as output', () => {
    renderCard(
      pricingModel({
        model_name: 'test-video',
        video_prices: {
          rows: [
            { resolution: '480p', normal_price: 0.05, off_peak_price: 0.03 },
            { resolution: '720p', normal_price: 0.1, off_peak_price: 0.07 },
          ],
        },
      })
    )

    expect(screen.getByText('Input')).toBeInTheDocument()
    expect(screen.getByText('-')).toBeInTheDocument()
    expect(screen.getByText('Output')).toBeInTheDocument()
    expect(screen.getByText(/From \$0\.05\/s/)).toBeInTheDocument()
    // per-resolution rows and the off-peak window move to the detail panel
    expect(screen.queryByText('480p')).not.toBeInTheDocument()
    expect(screen.queryByText(/Off-peak window/)).not.toBeInTheDocument()
  })

  test('chat dynamic pricing model keeps input and output entries', () => {
    renderCard(
      pricingModel({
        billing_mode: 'tiered_expr',
        billing_expr:
          'len <= 32000 ? tier("short", p * 1.5 + c * 6) : tier("long", p * 3 + c * 12)',
      })
    )

    expect(screen.getByText('Input')).toBeInTheDocument()
    expect(screen.getByText('$1.5')).toBeInTheDocument()
    expect(screen.getByText('Output')).toBeInTheDocument()
    expect(screen.getByText('$6')).toBeInTheDocument()
  })

  test('task dynamic pricing model keeps usage field prices instead of input/output', () => {
    renderCard(
      pricingModel({
        billing_mode: 'tiered_expr',
        billing_expr: 'tier("base", u("seconds") * 0.4)',
        billing_usage_schema: {
          seconds: { type: 'number', unit: 'second' },
        },
      })
    )

    expect(screen.getByText(/\$0\.4/)).toBeInTheDocument()
    expect(screen.queryByText('Input')).not.toBeInTheDocument()
    expect(screen.queryByText('Output')).not.toBeInTheDocument()
  })

  test('unconfigured task usage model keeps the not-configured notice', () => {
    renderCard(
      pricingModel({
        billing_usage_schema: {
          seconds: { type: 'number', unit: 'second' },
        },
      })
    )

    expect(
      screen.getByText('Usage-based billing · price not configured')
    ).toBeInTheDocument()
  })

  test('per-request model keeps the fixed price per request', () => {
    renderCard(pricingModel({ quota_type: 1, model_price: 0.5 }))

    expect(screen.getByText('$0.5')).toBeInTheDocument()
    expect(screen.getByText(/request/)).toBeInTheDocument()
  })
})
