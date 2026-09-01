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
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import {
  EMPTY_LANE_ENABLED,
  EMPTY_LANE_PRICES,
  buildPricingSubmitData,
  buildPreviewRows,
  getInitialPricingMode,
  type ModelPricingFormValues,
} from '../model-pricing-core'

const emptyValues: ModelPricingFormValues = {
  name: 'viduq3-pro',
  price: '',
  ratio: '',
  cacheRatio: '',
  createCacheRatio: '',
  completionRatio: '',
  imageRatio: '',
  audioRatio: '',
  audioCompletionRatio: '',
}

const videoTable = {
  rows: [
    { resolution: '1080p', normal_price: 0.75, off_peak_price: 0.375 },
    { resolution: '720p', normal_price: 0.625, off_peak_price: 0.3125 },
  ],
}

describe('getInitialPricingMode', () => {
  test('starts in video-per-second mode when the model has a video price table', () => {
    const mode = getInitialPricingMode({
      ...emptyValues,
      billingMode: 'video-per-second',
      videoPrices: videoTable,
    })
    assert.equal(mode, 'video-per-second')
  })

  test('starts in video-per-second mode when a table exists without an explicit mode', () => {
    const mode = getInitialPricingMode({
      ...emptyValues,
      videoPrices: videoTable,
    })
    assert.equal(mode, 'video-per-second')
  })

  test('keeps the existing tiered_expr, per-request and per-token detection', () => {
    assert.equal(
      getInitialPricingMode({ ...emptyValues, billingMode: 'tiered_expr' }),
      'tiered_expr'
    )
    assert.equal(
      getInitialPricingMode({ ...emptyValues, price: '0.01' }),
      'per-request'
    )
    assert.equal(getInitialPricingMode(emptyValues), 'per-token')
    assert.equal(getInitialPricingMode(null), 'per-token')
  })
})

describe('buildPricingSubmitData', () => {
  test('video-per-second payload carries the model video price table', () => {
    const data = buildPricingSubmitData(emptyValues, 'video-per-second', {
      billingExpr: '',
      requestRuleExpr: '',
      videoPrices: videoTable,
    })

    assert.equal(data.billingMode, 'video-per-second')
    assert.deepEqual(data.videoPrices, videoTable)
    assert.equal(data.billingExpr, undefined)
  })

  test('tiered_expr payload keeps the expression fields only', () => {
    const data = buildPricingSubmitData(emptyValues, 'tiered_expr', {
      billingExpr: 'tier("base", p * 2)',
      requestRuleExpr: '',
    })

    assert.equal(data.billingMode, 'tiered_expr')
    assert.equal(data.billingExpr, 'tier("base", p * 2)')
    assert.equal(data.videoPrices, undefined)
  })

  test('per-request payload carries the fixed price', () => {
    const data = buildPricingSubmitData(
      { ...emptyValues, price: '0.01' },
      'per-request',
      { billingExpr: '', requestRuleExpr: '' }
    )

    assert.equal(data.billingMode, 'per-request')
    assert.equal(data.price, '0.01')
    assert.equal(data.videoPrices, undefined)
  })
})

describe('buildPreviewRows video branch', () => {
  test('lists the configured resolutions for video-per-second mode', () => {
    const rows = buildPreviewRows(
      emptyValues,
      'video-per-second',
      '',
      '',
      '',
      EMPTY_LANE_PRICES,
      EMPTY_LANE_ENABLED,
      (key) => key,
      videoTable
    )

    assert.deepEqual(rows, [
      { key: 'mode', label: 'BillingMode', value: 'video-per-second' },
      { key: 'videoRows', label: 'Resolution', value: '1080p, 720p' },
    ])
  })

  test('shows empty state when no rows are configured', () => {
    const rows = buildPreviewRows(
      emptyValues,
      'video-per-second',
      '',
      '',
      '',
      EMPTY_LANE_PRICES,
      EMPTY_LANE_ENABLED,
      (key) => key,
      { rows: [] }
    )

    assert.equal(rows[1].value, 'Empty')
  })
})
