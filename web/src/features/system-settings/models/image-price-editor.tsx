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
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { FieldGroup } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from '@/components/ui/input-group'
import {
  DEFAULT_IMAGE_PRICE_RESOLUTION,
  imagePriceTierLabelKey,
} from '@/features/pricing/lib/image-price'
import type { ImagePriceTable } from '@/features/pricing/types'
import { useBillingCurrency } from '@/lib/currency'

import {
  addImagePriceResolutionDraft,
  defaultResolutionFromTable,
  imagePriceDraftModeFromTable,
  imagePriceDraftsFromTable,
  imagePriceTableFromDrafts,
  initialResolutionDrafts,
  rebaseImagePriceDrafts,
  removeImagePriceRowDraft,
  updateImagePriceRowDraft,
  type ImagePriceDraftMode,
  type ImagePriceRowDraft,
} from './image-price-drafts'
import { numericDraftRegex, usdPriceToDisplay } from './model-pricing-core'

export type ImagePriceEditorProps = {
  modelName?: string
  table: ImagePriceTable
  onChange: (table: ImagePriceTable) => void
}

/**
 * 图片按张计费编辑器。两种模式:
 * - 画质(Fast/Standard/High):固定三行,不可增删;
 * - 分辨率(WIDTHxHEIGHT):可增删行,并按请求 size 计价。
 * 价格为空的行/档位视为未定价,提交时不带该行。
 */
export const ImagePriceEditor = function ImagePriceEditor(
  props: ImagePriceEditorProps
) {
  const { t } = useTranslation()
  const { symbol: currencySymbol, exchangeRate } = useBillingCurrency()
  const [mode, setMode] = useState<ImagePriceDraftMode>(() =>
    imagePriceDraftModeFromTable(props.table)
  )
  const [drafts, setDrafts] = useState<ImagePriceRowDraft[]>(() =>
    imagePriceDraftsFromTable(props.table)
  )
  const [defaultResolution, setDefaultResolution] = useState(() =>
    defaultResolutionFromTable(props.table)
  )
  // 草稿按加载时的汇率换算成显示货币;汇率变化时 rebase(保留底层 USD 意图),
  // 否则后续任一行的编辑会按新汇率重换算整张旧汇率草稿,写错未编辑行的价。
  const draftRateRef = useRef(exchangeRate)

  useEffect(() => {
    const nextRate = exchangeRate
    const prevRate = draftRateRef.current
    if (nextRate === prevRate) return
    draftRateRef.current = nextRate
    setDrafts((prev) => rebaseImagePriceDrafts(prev, prevRate, nextRate))
  }, [exchangeRate])

  const emitChange = (
    nextDrafts: ImagePriceRowDraft[],
    nextMode: ImagePriceDraftMode = mode,
    nextDefaultResolution: string = defaultResolution
  ) => {
    setDrafts(nextDrafts)
    setMode(nextMode)
    setDefaultResolution(nextDefaultResolution)
    props.onChange(
      imagePriceTableFromDrafts(nextDrafts, {
        mode: nextMode,
        defaultResolution:
          nextMode === 'resolution' ? nextDefaultResolution : undefined,
      })
    )
  }

  const handleRowChange = (id: string, patch: Partial<ImagePriceRowDraft>) => {
    const index = drafts.findIndex((draft) => draft.id === id)
    if (index === -1) return
    emitChange(updateImagePriceRowDraft(drafts, index, patch))
  }

  const switchMode = (nextMode: ImagePriceDraftMode) => {
    if (nextMode === mode) return
    if (nextMode === 'resolution') {
      const standardPrice =
        drafts.find((draft) => draft.tier === 'standard')?.price ?? ''
      emitChange(
        initialResolutionDrafts().map((draft) => ({
          ...draft,
          price: standardPrice,
        })),
        'resolution',
        DEFAULT_IMAGE_PRICE_RESOLUTION
      )
      return
    }
    const resolutionPrice =
      drafts.find(
        (draft) => draft.resolution === DEFAULT_IMAGE_PRICE_RESOLUTION
      )?.price ?? ''
    const qualityDrafts = imagePriceDraftsFromTable({ rows: [] }).map((draft) =>
      draft.tier === 'standard' ? { ...draft, price: resolutionPrice } : draft
    )
    emitChange(qualityDrafts, 'quality', DEFAULT_IMAGE_PRICE_RESOLUTION)
  }

  const rowGridClass =
    'grid grid-cols-[minmax(0,1fr)_minmax(100px,150px)] items-center gap-2'
  const resolutionRowGridClass =
    'grid grid-cols-[minmax(0,1fr)_minmax(100px,150px)_auto] items-center gap-2'

  return (
    <FieldGroup className='gap-4'>
      <div className='flex flex-wrap items-center gap-2'>
        <span className='text-muted-foreground text-xs'>
          {t('Pricing mode')}
        </span>
        <Button
          type='button'
          size='sm'
          variant={mode === 'quality' ? 'default' : 'outline'}
          onClick={() => switchMode('quality')}
        >
          {t('Quality tiers')}
        </Button>
        <Button
          type='button'
          size='sm'
          variant={mode === 'resolution' ? 'default' : 'outline'}
          onClick={() => switchMode('resolution')}
        >
          {t('Resolution')}
        </Button>
      </div>

      {mode === 'quality' ? (
        <div className='space-y-2'>
          <div className={rowGridClass}>
            <span className='text-muted-foreground text-xs'>
              {t('Quality')}
            </span>
            <span className='text-muted-foreground text-xs'>
              {t('Image price ({{symbol}}/image)', { symbol: currencySymbol })}
            </span>
          </div>
          {drafts.map((draft) => {
            const labelKey = imagePriceTierLabelKey(draft.tier)
            return (
              <div key={draft.id} className={rowGridClass}>
                <span className='text-sm'>
                  {labelKey ? t(labelKey) : draft.tier}
                </span>
                <InputGroup>
                  <InputGroupAddon>{currencySymbol}</InputGroupAddon>
                  <InputGroupInput
                    inputMode='decimal'
                    value={draft.price}
                    placeholder={usdPriceToDisplay(0.02)}
                    onChange={(event) => {
                      const value = event.target.value
                      if (numericDraftRegex.test(value)) {
                        handleRowChange(draft.id, { price: value })
                      }
                    }}
                  />
                </InputGroup>
              </div>
            )
          })}
        </div>
      ) : (
        <div className='space-y-2'>
          <div className={rowGridClass}>
            <span className='text-muted-foreground text-xs'>
              {t('Default resolution')}
            </span>
            <Input
              value={defaultResolution}
              placeholder={DEFAULT_IMAGE_PRICE_RESOLUTION}
              onChange={(event) => {
                emitChange(drafts, 'resolution', event.target.value)
              }}
            />
          </div>
          <div className={resolutionRowGridClass}>
            <span className='text-muted-foreground text-xs'>
              {t('Resolution')}
            </span>
            <span className='text-muted-foreground text-xs'>
              {t('Image price ({{symbol}}/image)', { symbol: currencySymbol })}
            </span>
            <span />
          </div>
          {drafts.map((draft) => (
            <div key={draft.id} className={resolutionRowGridClass}>
              <Input
                value={draft.resolution ?? ''}
                placeholder={DEFAULT_IMAGE_PRICE_RESOLUTION}
                onChange={(event) => {
                  handleRowChange(draft.id, { resolution: event.target.value })
                }}
              />
              <InputGroup>
                <InputGroupAddon>{currencySymbol}</InputGroupAddon>
                <InputGroupInput
                  inputMode='decimal'
                  value={draft.price}
                  placeholder={usdPriceToDisplay(0.02)}
                  onChange={(event) => {
                    const value = event.target.value
                    if (numericDraftRegex.test(value)) {
                      handleRowChange(draft.id, { price: value })
                    }
                  }}
                />
              </InputGroup>
              <Button
                type='button'
                size='sm'
                variant='ghost'
                onClick={() =>
                  emitChange(removeImagePriceRowDraft(drafts, draft.id))
                }
              >
                {t('Delete')}
              </Button>
            </div>
          ))}
          <Button
            type='button'
            size='sm'
            variant='outline'
            onClick={() => emitChange(addImagePriceResolutionDraft(drafts))}
          >
            {t('Add resolution')}
          </Button>
        </div>
      )}
    </FieldGroup>
  )
}
