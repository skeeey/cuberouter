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

import type { OffPeakWindow } from '@/features/pricing/types'

import {
  draftsToWindow,
  hourDraftRegex,
  isOffPeakDisabled,
  isOffPeakWindowJson,
  isValidHourDraft,
  parseOffPeakWindowJson,
  windowToDrafts,
} from '../off-peak-window-drafts'

const defaultWindow: OffPeakWindow = {
  start_hour: 22,
  end_hour: 8,
  timezone: 'Asia/Shanghai',
}

describe('off-peak window drafts', () => {
  test('window to drafts preserves values as strings', () => {
    assert.deepEqual(windowToDrafts(defaultWindow), {
      startHour: '22',
      endHour: '8',
      timezone: 'Asia/Shanghai',
    })
  })

  test('drafts to window round-trips the default window', () => {
    const window = draftsToWindow(windowToDrafts(defaultWindow))
    assert.deepEqual(window, defaultWindow)
  })

  test('drafts to window returns null for an empty or out-of-range hour', () => {
    assert.equal(draftsToWindow({ startHour: '', endHour: '8', timezone: 'x' }), null)
    assert.equal(draftsToWindow({ startHour: '24', endHour: '8', timezone: 'x' }), null)
    assert.equal(draftsToWindow({ startHour: '-1', endHour: '8', timezone: 'x' }), null)
    assert.equal(draftsToWindow({ startHour: '22', endHour: '1a', timezone: 'x' }), null)
  })

  test('drafts to window trims the timezone', () => {
    const window = draftsToWindow({
      startHour: '22',
      endHour: '8',
      timezone: '  Asia/Shanghai  ',
    })
    assert.equal(window?.timezone, 'Asia/Shanghai')
  })

  test('hour draft regex only admits up to two digits', () => {
    assert.ok(hourDraftRegex.test(''))
    assert.ok(hourDraftRegex.test('0'))
    assert.ok(hourDraftRegex.test('23'))
    assert.equal(hourDraftRegex.test('234'), false)
    assert.equal(hourDraftRegex.test('2a'), false)
    assert.equal(hourDraftRegex.test(' 2'), false)
  })

  test('hour validation accepts 0-23 and rejects empty, negative or out-of-range', () => {
    assert.equal(isValidHourDraft('0'), true)
    assert.equal(isValidHourDraft('23'), true)
    assert.equal(isValidHourDraft(''), false)
    assert.equal(isValidHourDraft('24'), false)
    assert.equal(isValidHourDraft('-1'), false)
    assert.equal(isValidHourDraft('1.5'), false)
  })

  test('equal start and end hours disable off-peak pricing', () => {
    assert.equal(
      isOffPeakDisabled({ start_hour: 22, end_hour: 22, timezone: 'UTC' }),
      true
    )
    assert.equal(isOffPeakDisabled(defaultWindow), false)
  })

  test('parse window JSON accepts valid and lenient numbers', () => {
    assert.deepEqual(
      parseOffPeakWindowJson(JSON.stringify(defaultWindow)),
      defaultWindow
    )
    assert.deepEqual(
      parseOffPeakWindowJson('{"start_hour":24,"end_hour":8,"timezone":"UTC"}'),
      { start_hour: 24, end_hour: 8, timezone: 'UTC' }
    )
  })

  test('parse window JSON rejects malformed input', () => {
    assert.equal(parseOffPeakWindowJson(''), null)
    assert.equal(parseOffPeakWindowJson('  '), null)
    assert.equal(parseOffPeakWindowJson('{nope'), null)
    assert.equal(parseOffPeakWindowJson('[]'), null)
    assert.equal(
      parseOffPeakWindowJson('{"start_hour":"22","end_hour":8,"timezone":"UTC"}'),
      null
    )
  })

  test('window JSON structure check requires integer hours in [0,23] and a timezone string', () => {
    assert.equal(isOffPeakWindowJson(defaultWindow), true)
    assert.equal(
      isOffPeakWindowJson({ start_hour: 0, end_hour: 23, timezone: '' }),
      true
    )
    assert.equal(
      isOffPeakWindowJson({ start_hour: 24, end_hour: 8, timezone: 'UTC' }),
      false
    )
    assert.equal(
      isOffPeakWindowJson({ start_hour: 22, end_hour: 8 }),
      false
    )
    assert.equal(
      isOffPeakWindowJson({ start_hour: 22.5, end_hour: 8, timezone: 'UTC' }),
      false
    )
    assert.equal(isOffPeakWindowJson(null), false)
    assert.equal(isOffPeakWindowJson('not an object'), false)
  })
})
