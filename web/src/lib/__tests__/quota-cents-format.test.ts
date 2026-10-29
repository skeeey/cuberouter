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
import { beforeEach, describe, expect, it } from 'vitest'

import { formatQuotaToCents } from '@/lib/currency'
import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
  type CurrencyDisplayType,
} from '@/stores/system-config-store'

function setDisplay(type: CurrencyDisplayType, rate = 1): void {
  useSystemConfigStore.setState((state) => ({
    config: {
      ...state.config,
      currency: {
        ...DEFAULT_CURRENCY_CONFIG,
        quotaDisplayType: type,
        usdExchangeRate: rate,
        customCurrencyExchangeRate: rate,
      },
    },
  }))
}

beforeEach(() => {
  useSystemConfigStore.setState((state) => ({
    config: { ...state.config, currency: { ...DEFAULT_CURRENCY_CONFIG } },
  }))
})

describe('formatQuotaToCents', () => {
  it('USD keeps exactly two fraction digits', () => {
    setDisplay('USD')

    expect(formatQuotaToCents(500000)).toContain('1.00')
    expect(formatQuotaToCents(505000)).toContain('1.01')
    expect(formatQuotaToCents(0)).toContain('0.00')
  })

  it('CNY converts by the exchange rate and keeps two fraction digits', () => {
    setDisplay('CNY', 7.3)

    expect(formatQuotaToCents(500000)).toContain('7.30')
  })

  it('CUSTOM uses the configured symbol', () => {
    setDisplay('CUSTOM', 2.5)

    expect(formatQuotaToCents(500000)).toContain('¤ 2.50')
  })

  it('TOKENS keeps the normal quota format', () => {
    setDisplay('TOKENS')

    expect(formatQuotaToCents(500000)).toBe('500k')
  })
})
