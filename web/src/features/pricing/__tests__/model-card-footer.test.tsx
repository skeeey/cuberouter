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
// externalized ESM loader rejects. Icon rendering is irrelevant to the layout
// contracts under test, so the icon loader boundary is stubbed.
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

describe('ModelCard footer meta', () => {
  test('shows every endpoint with its display label instead of the raw type', () => {
    renderCard(
      pricingModel({
        supported_endpoint_types: ['openai', 'openai-response', 'anthropic'],
      })
    )

    expect(screen.getByText('Chat')).toBeInTheDocument()
    expect(screen.getByText('Response')).toBeInTheDocument()
    expect(screen.getByText('Anthropic')).toBeInTheDocument()
    expect(screen.queryByText('openai')).not.toBeInTheDocument()
  })

  test('reads a per-second video model as the single video endpoint', () => {
    renderCard(
      pricingModel({
        supported_endpoint_types: ['openai'],
        video_prices: {
          rows: [
            { resolution: '720p', normal_price: 0.1, off_peak_price: 0.07 },
          ],
        },
      })
    )

    expect(screen.getByText('Video')).toBeInTheDocument()
    expect(screen.queryByText('Chat')).not.toBeInTheDocument()
  })

  test('lays tags and endpoints out as separate rows', () => {
    renderCard(
      pricingModel({
        tags: 'coding,agent',
        supported_endpoint_types: ['openai', 'anthropic'],
      })
    )

    const tagsRow = screen.getByText('coding').parentElement
    const endpointsRow = screen.getByText('Chat').parentElement

    expect(tagsRow).not.toBe(endpointsRow)
    expect([...(tagsRow?.children ?? [])].map((el) => el.textContent)).toEqual([
      'coding',
      'agent',
    ])
    expect(
      [...(endpointsRow?.children ?? [])].map((el) => el.textContent)
    ).toEqual(['Chat', 'Anthropic'])
  })

  test('shows at most four tags and no hidden-count badge', () => {
    renderCard(
      pricingModel({
        tags: 'alpha,beta,gamma,delta,epsilon,zeta',
        quota_type: 1,
        model_price: 0.5,
      })
    )

    expect(screen.getByText('alpha')).toBeInTheDocument()
    expect(screen.getByText('delta')).toBeInTheDocument()
    expect(screen.queryByText('epsilon')).not.toBeInTheDocument()
    expect(screen.queryByText('zeta')).not.toBeInTheDocument()
    expect(screen.queryByText('+2')).not.toBeInTheDocument()
  })

  test('drops the token unit label from the footer', () => {
    renderCard(pricingModel({}))

    expect(screen.queryByText('1M')).not.toBeInTheDocument()
    expect(screen.queryByText('1K')).not.toBeInTheDocument()
  })
  test('leaves the group and billing mode off the card', () => {
    renderCard(pricingModel({ tags: 'coding' }))

    expect(screen.queryByText('default')).not.toBeInTheDocument()
    expect(screen.queryByText('Token-based')).not.toBeInTheDocument()
    expect(
      screen
        .getByRole('heading', { name: 'test-model' })
        .parentElement?.querySelector('[data-slot="status-badge"]')
    ).toBeNull()
  })
})
