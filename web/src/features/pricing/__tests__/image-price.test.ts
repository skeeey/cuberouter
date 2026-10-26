/*
Copyright (C) 2023-2026 QuantumNous
Copyright (C) 2026 CubeRouter

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
import { describe, expect, test } from 'vitest'

import {
  imagePriceTierLabelKey,
  isResolutionImagePriceTable,
} from '../lib/image-price'

describe('image price table mode', () => {
  test('quality tiers are not resolution-based', () => {
    expect(
      isResolutionImagePriceTable({
        rows: [{ tier: 'standard', price: 0.006 }],
      })
    ).toBe(false)
    expect(imagePriceTierLabelKey('standard')).toBe('Standard')
    expect(imagePriceTierLabelKey(undefined)).toBeUndefined()
  })

  test('a row with resolution makes the table resolution-based', () => {
    expect(
      isResolutionImagePriceTable({
        default_resolution: '1024x1024',
        rows: [{ resolution: '1024x1024', price: 0.006 }],
      })
    ).toBe(true)
    expect(imagePriceTierLabelKey('1024x1024')).toBeUndefined()
  })
})
