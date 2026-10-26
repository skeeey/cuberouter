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
import { beforeEach, describe, expect, test } from 'vitest'

import type { ImagePriceTable } from '@/features/pricing/types'
import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
  type CurrencyDisplayType,
} from '@/stores/system-config-store'

import {
  addImagePriceResolutionDraft,
  initialResolutionDrafts,
  rebaseImagePriceDrafts,
  removeImagePriceRowDraft,
  updateImagePriceRowDraft,
  imagePriceDraftsFromTable,
  imagePriceTableFromDrafts,
  type ImagePriceRowDraft,
} from '../image-price-drafts'

// 注水方式与 model-pricing-core.test.ts 一致(store 真实类型,currency 不可为 null)。
function seedDisplayCurrency(type: CurrencyDisplayType, rate: number): void {
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

function resetDisplayCurrency(): void {
  useSystemConfigStore.setState((state) => ({
    config: { ...state.config, currency: { ...DEFAULT_CURRENCY_CONFIG } },
  }))
}

function draft(
  overrides: Partial<ImagePriceRowDraft> = {}
): ImagePriceRowDraft {
  return { id: 'id', tier: 'fast', price: '', ...overrides }
}

describe('image price drafts currency boundary (CNY rate 7.3)', () => {
  beforeEach(() => seedDisplayCurrency('CNY', 7.3))

  test('加载:固定三档,已配置档位按汇率转为显示货币草稿,未配置档为空', () => {
    const drafts = imagePriceDraftsFromTable({
      rows: [
        { tier: 'fast', price: 0.75 },
        { tier: 'high', price: 1.5 },
      ],
    })

    expect(drafts.map((entry) => entry.tier)).toEqual([
      'fast',
      'standard',
      'high',
    ])
    const [fast, standard, high] = drafts
    expect(typeof fast.id).toBe('string')
    expect(fast.price).toBe('5.475')
    expect(standard.price).toBe('')
    expect(high.price).toBe('10.95')
  })

  test('提交:显示货币草稿字符串按汇率转为 USD/张 表载荷,空档被过滤', () => {
    const table = imagePriceTableFromDrafts([
      draft({ tier: 'fast', price: '5.475' }),
      draft({ tier: 'standard' }),
      draft({ tier: 'high' }),
    ])

    expect(table.rows).toEqual([{ tier: 'fast', price: 0.75 }])
  })

  test('往返:USD/张 表 → 显示草稿 → USD/张 表数值稳定', () => {
    const original: ImagePriceTable = {
      rows: [
        { tier: 'fast', price: 0.75 },
        { tier: 'high', price: 1.5 },
      ],
    }

    const drafts = imagePriceDraftsFromTable(original)

    expect(imagePriceTableFromDrafts(drafts)).toEqual(original)
  })

  test('空/非法价格草稿仍归零(0 ÷ 汇率 = 0)', () => {
    const table = imagePriceTableFromDrafts([draft({ price: 'abc' })])

    expect(table.rows[0].price).toBe(0)
  })
})

describe('image price drafts currency boundary (USD)', () => {
  beforeEach(resetDisplayCurrency)

  test('加载/提交均为恒等,与旧行为一致', () => {
    const drafts = imagePriceDraftsFromTable({
      rows: [{ tier: 'fast', price: 0.75 }],
    })

    expect(drafts[0].price).toBe('0.75')

    const table = imagePriceTableFromDrafts([
      draft({ price: '0.75' }),
      draft({ tier: 'standard' }),
      draft({ tier: 'high' }),
    ])

    expect(table.rows).toEqual([{ tier: 'fast', price: 0.75 }])
  })
})

describe('image price draft list helpers', () => {
  beforeEach(resetDisplayCurrency)

  test('editing a tier price emits only the priced tiers', () => {
    let drafts = imagePriceDraftsFromTable({ rows: [] })
    expect(drafts.length).toBe(3)
    expect(drafts.every((entry) => entry.price === '')).toBe(true)

    drafts = updateImagePriceRowDraft(drafts, 0, { price: '0.75' })

    const table = imagePriceTableFromDrafts(drafts)
    expect(table).toEqual({ rows: [{ tier: 'fast', price: 0.75 }] })
  })

  test('keeps an explicit zero price as a valid draft value', () => {
    const table = imagePriceTableFromDrafts([draft({ price: '0' })])

    expect(table.rows).toEqual([{ tier: 'fast', price: 0 }])
  })
})

describe('rebaseImagePriceDrafts(汇率变化保留底层 USD 意图)', () => {
  test('CNY 7.3 → USD 1:显示草稿回落到 USD 数值', () => {
    const drafts = [
      draft({ tier: 'fast', price: '5.475' }),
      draft({ tier: 'standard', price: '' }),
    ]

    const rebased = rebaseImagePriceDrafts(drafts, 7.3, 1)

    expect(rebased[0].tier).toBe('fast')
    expect(rebased[0].price).toBe('0.75')
    // 空档(未录入)原样保留
    expect(rebased[1].price).toBe('')
  })

  test('USD 1 → CNY 7.3:显示草稿按新汇率放大,保持同一美元意图', () => {
    const drafts = [draft({ tier: 'standard', price: '0.75' })]

    const rebased = rebaseImagePriceDrafts(drafts, 1, 7.3)

    expect(rebased[0].price).toBe('5.475')
  })

  test('汇率相等时恒等返回,不重建对象', () => {
    const drafts = [draft({ price: '5.475' })]
    expect(rebaseImagePriceDrafts(drafts, 7.3, 7.3)).toBe(drafts)
  })

  test('不可解析的录入串(空/尾点)原样保留,不打断输入', () => {
    const drafts = [draft({ tier: 'fast', price: '.' })]

    const rebased = rebaseImagePriceDrafts(drafts, 7.3, 1)

    expect(rebased[0].price).toBe('.')
  })
})

describe('image price drafts resolution tables', () => {
  beforeEach(() => seedDisplayCurrency('CNY', 7.3))

  test('加载:分辨率行按汇率转为显示货币,并保留分辨率', () => {
    const drafts = imagePriceDraftsFromTable({
      default_resolution: '1024x1024',
      rows: [
        { resolution: '1024x1024', price: 0.75 },
        { resolution: '1536x1024', price: 1.5 },
      ],
    })

    expect(drafts.map((entry) => entry.resolution)).toEqual([
      '1024x1024',
      '1536x1024',
    ])
    expect(drafts.map((entry) => entry.price)).toEqual(['5.475', '10.95'])
  })

  test('提交:分辨率草稿生成 rows + default_resolution', () => {
    const table = imagePriceTableFromDrafts(
      [
        draft({ tier: undefined, resolution: ' 1024X1024 ', price: '5.475' }),
        draft({ tier: undefined, resolution: '1536x1024', price: '10.95' }),
        draft({ tier: undefined, resolution: '', price: '1' }),
      ],
      { mode: 'resolution', defaultResolution: '1024X1024' }
    )

    expect(table).toEqual({
      default_resolution: '1024x1024',
      rows: [
        { resolution: '1024x1024', price: 0.75 },
        { resolution: '1536x1024', price: 1.5 },
      ],
    })
  })

  test('往返:分辨率表 → 草稿 → 表数值稳定', () => {
    const original: ImagePriceTable = {
      default_resolution: '1024x1024',
      rows: [
        { resolution: '1024x1024', price: 0.75 },
        { resolution: '1536x1024', price: 1.5 },
      ],
    }

    const drafts = imagePriceDraftsFromTable(original)

    expect(
      imagePriceTableFromDrafts(drafts, {
        mode: 'resolution',
        defaultResolution: original.default_resolution,
      })
    ).toEqual(original)
  })

  test('分辨率行增删 helper', () => {
    const initial = initialResolutionDrafts()
    expect(initial).toHaveLength(1)
    expect(initial[0].resolution).toBe('1024x1024')

    const added = addImagePriceResolutionDraft(initial)
    expect(added).toHaveLength(2)
    expect(added[1].resolution).toBe('')

    const removed = removeImagePriceRowDraft(added, added[0].id)
    expect(removed).toHaveLength(1)
    expect(removed[0].id).toBe(added[1].id)
  })
})
