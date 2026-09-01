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
  addVideoPriceRowDraft,
  createVideoPriceRowDraft,
  removeVideoPriceRowDraft,
  updateVideoPriceRowDraft,
  videoPriceDraftsFromTable,
  videoPriceTableFromDrafts,
  type VideoPriceRowDraft,
} from '../video-price-drafts'

function draft(overrides: Partial<VideoPriceRowDraft> = {}): VideoPriceRowDraft {
  return { ...createVideoPriceRowDraft(), ...overrides }
}

describe('video price editor drafts', () => {
  test('maps a saved table to editable drafts preserving values', () => {
    const drafts = videoPriceDraftsFromTable({
      rows: [
        { resolution: '1080p', normal_price: 0.75, off_peak_price: 0.375 },
      ],
    })

    assert.equal(drafts.length, 1)
    const [row] = drafts
    assert.equal(typeof row.id, 'string')
    assert.equal(row.resolution, '1080p')
    assert.equal(row.normalPrice, '0.75')
    assert.equal(row.offPeakPrice, '0.375')
  })

  test('adding a row and editing values emits a table with the filled row', () => {
    let drafts = addVideoPriceRowDraft([])
    assert.equal(drafts.length, 1)
    assert.equal(drafts[0].resolution, '')
    assert.equal(drafts[0].normalPrice, '')
    assert.equal(drafts[0].offPeakPrice, '')

    drafts = addVideoPriceRowDraft(drafts)
    drafts = updateVideoPriceRowDraft(drafts, 0, {
      resolution: '1080p',
      normalPrice: '0.75',
      offPeakPrice: '0.375',
    })

    const table = videoPriceTableFromDrafts(drafts)
    assert.deepEqual(table, {
      rows: [
        { resolution: '1080p', normal_price: 0.75, off_peak_price: 0.375 },
      ],
    })
  })

  test('removing a row drops it from the emitted table', () => {
    let drafts = videoPriceDraftsFromTable({
      rows: [
        { resolution: '1080p', normal_price: 0.75, off_peak_price: 0.375 },
        { resolution: '720p', normal_price: 0.625, off_peak_price: 0.3125 },
      ],
    })

    drafts = removeVideoPriceRowDraft(drafts, 0)

    const table = videoPriceTableFromDrafts(drafts)
    assert.deepEqual(table.rows, [
      { resolution: '720p', normal_price: 0.625, off_peak_price: 0.3125 },
    ])
  })

  test('trims resolution text when emitting the table', () => {
    const table = videoPriceTableFromDrafts([
      draft({ resolution: ' 1080p ', normalPrice: '0.75', offPeakPrice: '0.5' }),
    ])

    assert.equal(table.rows[0].resolution, '1080p')
    assert.equal(table.rows[0].normal_price, 0.75)
  })

  test('keeps partially filled rows for backend validation', () => {
    const table = videoPriceTableFromDrafts([
      draft({ resolution: '4K' }),
      draft(),
    ])

    assert.deepEqual(table.rows, [
      { resolution: '4K', normal_price: 0, off_peak_price: 0 },
    ])
  })

  test('keeps an explicit zero off-peak price as a valid draft value', () => {
    const table = videoPriceTableFromDrafts([
      draft({ resolution: '1080p', normalPrice: '0.75', offPeakPrice: '0' }),
    ])

    assert.deepEqual(table.rows, [
      { resolution: '1080p', normal_price: 0.75, off_peak_price: 0 },
    ])
  })
})
