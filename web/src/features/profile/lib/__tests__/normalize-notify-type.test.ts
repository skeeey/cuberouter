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
import { describe, test } from 'vitest'

import { normalizeNotifyType } from '../format'

describe('normalizeNotifyType', () => {
  // The notification selector only offers Email, but a profile that already
  // stores one of the other methods keeps it: the settings form re-saves
  // whatever it loaded, so normalizing those to "email" would silently switch
  // the user's delivery channel on the next save.
  test('keeps notification methods the selector no longer offers', () => {
    assert.equal(normalizeNotifyType('webhook'), 'webhook')
    assert.equal(normalizeNotifyType('bark'), 'bark')
    assert.equal(normalizeNotifyType('gotify'), 'gotify')
  })

  test('keeps email and falls back to it for missing or unknown values', () => {
    assert.equal(normalizeNotifyType('email'), 'email')
    assert.equal(normalizeNotifyType('telegram'), 'email')
    assert.equal(normalizeNotifyType(undefined), 'email')
    assert.equal(normalizeNotifyType(''), 'email')
    assert.equal(normalizeNotifyType(3), 'email')
  })
})
