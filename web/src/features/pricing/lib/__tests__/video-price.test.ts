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
  formatOffPeakHour,
  formatVideoPrice,
  getOffPeakWindowLabel,
} from '../video-price'

describe('formatVideoPrice', () => {
  test('renders configured values verbatim without trailing zeros', () => {
    assert.equal(formatVideoPrice(0.75), '0.75')
    assert.equal(formatVideoPrice(0.375), '0.375')
    assert.equal(formatVideoPrice(0.625), '0.625')
    assert.equal(formatVideoPrice(0.3125), '0.3125')
    assert.equal(formatVideoPrice(1), '1')
    assert.equal(formatVideoPrice(0), '0')
  })

  test('falls back to a placeholder for non-finite values', () => {
    assert.equal(formatVideoPrice(Number.NaN), '—')
    assert.equal(formatVideoPrice(Number.POSITIVE_INFINITY), '—')
  })
})

describe('formatOffPeakHour', () => {
  test('formats hours with zero padding', () => {
    assert.equal(formatOffPeakHour(22), '22:00')
    assert.equal(formatOffPeakHour(8), '08:00')
    assert.equal(formatOffPeakHour(0), '00:00')
    assert.equal(formatOffPeakHour(23), '23:00')
  })

  test('rejects out-of-range or fractional hours', () => {
    assert.equal(formatOffPeakHour(-1), '—')
    assert.equal(formatOffPeakHour(24), '—')
    assert.equal(formatOffPeakHour(12.5), '—')
    assert.equal(formatOffPeakHour(Number.NaN), '—')
  })
})

describe('getOffPeakWindowLabel', () => {
  test('flags a window crossing midnight', () => {
    assert.deepEqual(
      getOffPeakWindowLabel({
        start_hour: 22,
        end_hour: 8,
        timezone: 'Asia/Shanghai',
      }),
      { start: '22:00', end: '08:00', crossesMidnight: true }
    )
  })

  test('does not flag a same-day window', () => {
    assert.deepEqual(
      getOffPeakWindowLabel({
        start_hour: 9,
        end_hour: 17,
        timezone: 'Asia/Shanghai',
      }),
      { start: '09:00', end: '17:00', crossesMidnight: false }
    )
  })

  test('equal start and end hours do not count as crossing midnight', () => {
    const label = getOffPeakWindowLabel({
      start_hour: 22,
      end_hour: 22,
      timezone: 'Asia/Shanghai',
    })
    assert.equal(label?.crossesMidnight, false)
  })

  test('returns null when the window is missing or invalid', () => {
    assert.equal(getOffPeakWindowLabel(undefined), null)
    assert.equal(
      getOffPeakWindowLabel({
        start_hour: 24,
        end_hour: 8,
        timezone: 'Asia/Shanghai',
      }),
      null
    )
    assert.equal(
      getOffPeakWindowLabel({
        start_hour: 22,
        end_hour: 24,
        timezone: 'Asia/Shanghai',
      }),
      null
    )
  })
})
