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
import type { TFunction } from 'i18next'

import type { TokenUnit } from './types'

// ----------------------------------------------------------------------------
// Pricing Constants
// ----------------------------------------------------------------------------

/** Sort options for pricing models */
export const SORT_OPTIONS = {
  NAME: 'name',
  PRICE_LOW: 'price-low',
  PRICE_HIGH: 'price-high',
} as const

/**
 * Whether the model square offers a sort control at all.
 *
 * Hidden for now: with the price options withdrawn (SHOW_PRICE_SORT) only the
 * name sort was left, which did not earn a control of its own. The sort labels,
 * the sort state and the `?sort=` parameter all stay wired, so flipping this
 * brings the control back — with just the name option until SHOW_PRICE_SORT is
 * flipped too.
 */
export const SHOW_SORT_CONTROL = false

/**
 * Whether to offer the price sort options (low → high / high → low).
 *
 * They are hidden for now: the current sort key does not describe what the
 * cards show, and a rework is pending. See the TODO on getModelPrice in
 * lib/filters.ts for the details and the agreed direction. This only has an
 * effect once SHOW_SORT_CONTROL is true again.
 */
export const SHOW_PRICE_SORT = false

export type SortOption = (typeof SORT_OPTIONS)[keyof typeof SORT_OPTIONS]

export function getSortLabels(t: TFunction): Record<SortOption, string> {
  return {
    [SORT_OPTIONS.NAME]: t('Name'),
    [SORT_OPTIONS.PRICE_LOW]: t('Price: Low to High'),
    [SORT_OPTIONS.PRICE_HIGH]: t('Price: High to Low'),
  }
}

/** Filter values */
export const FILTER_ALL = 'all'

/** Quota type options */
export const QUOTA_TYPES = {
  ALL: 'all',
  TOKEN: 'token',
  REQUEST: 'request',
  TASK: 'task',
} as const

export type QuotaTypeOption = (typeof QUOTA_TYPES)[keyof typeof QUOTA_TYPES]

/** Quota type labels */
export function getQuotaTypeLabels(
  t: TFunction
): Record<QuotaTypeOption, string> {
  return {
    [QUOTA_TYPES.ALL]: t('All Models'),
    [QUOTA_TYPES.TOKEN]: t('Token-based'),
    [QUOTA_TYPES.REQUEST]: t('Per Request'),
    [QUOTA_TYPES.TASK]: t('Task billing'),
  }
}

/** Endpoint type options */
export const ENDPOINT_TYPES = {
  ALL: 'all',
  OPENAI: 'openai',
  OPENAI_RESPONSE: 'openai-response',
  ANTHROPIC: 'anthropic',
  GEMINI: 'gemini',
  JINA_RERANK: 'jina-rerank',
  IMAGE_GENERATION: 'image-generation',
  EMBEDDINGS: 'embeddings',
  OPENAI_VIDEO: 'openai-video',
  ARK_VIDEO: 'ark-video',
  /**
   * Filter value standing for both video endpoint styles — Sora / doubao
   * (openai-video) and AstraFlow (ark-video). No model carries this value in
   * its `supported_endpoint_types`; matchesEndpointType (lib/filters.ts) is
   * what expands it. Both styles mean "this model generates video", and the
   * style a caller ends up using depends on the path they call, so the
   * marketplace offers a single Video filter.
   */
  VIDEO: 'video',
} as const

export type EndpointTypeOption =
  (typeof ENDPOINT_TYPES)[keyof typeof ENDPOINT_TYPES]

/** Endpoint values the filter panel offers: video is listed once for both styles. */
export type EndpointFilterValue = Exclude<
  EndpointTypeOption,
  'openai-video' | 'ark-video'
>

/**
 * Endpoint type labels — also the source of the filter panel's options, so the
 * two video styles are listed once, under VIDEO. The individual OPENAI_VIDEO /
 * ARK_VIDEO values stay for matching (see ENDPOINT_TYPES.VIDEO).
 */
export function getEndpointTypeLabels(
  t: TFunction
): Record<EndpointFilterValue, string> {
  return {
    [ENDPOINT_TYPES.ALL]: t('All Types'),
    [ENDPOINT_TYPES.OPENAI]: 'Chat',
    [ENDPOINT_TYPES.OPENAI_RESPONSE]: 'Response',
    [ENDPOINT_TYPES.ANTHROPIC]: 'Anthropic',
    [ENDPOINT_TYPES.GEMINI]: 'Gemini',
    [ENDPOINT_TYPES.JINA_RERANK]: 'Rerank',
    [ENDPOINT_TYPES.IMAGE_GENERATION]: t('Image'),
    [ENDPOINT_TYPES.EMBEDDINGS]: t('Embeddings'),
    [ENDPOINT_TYPES.VIDEO]: t('Video'),
  }
}

/**
 * Whether the filter panel offers the group filter and the pricing-type
 * (per-token / per-request / task) filter.
 *
 * Both are withdrawn for now. The option lists, their counts and the filter
 * state stay wired, and the values are ignored while hidden (use-filters.ts), so
 * a shared ?group=... or ?quotaType=... link cannot filter the list with no
 * control on screen to show or clear it.
 */
export const SHOW_GROUP_FILTER = false
export const SHOW_QUOTA_TYPE_FILTER = false

/** Filter section keys */
export const FILTER_SECTIONS = {
  PRICING_TYPE: 'pricingType',
  ENDPOINT_TYPE: 'endpointType',
  VENDOR: 'vendor',
  GROUP: 'group',
  TAG: 'tag',
} as const

/** Maximum number of tags to display in model row */
export const MAX_TAGS_DISPLAY = 5

/** Maximum number of filter items to display before showing "More..." */
export const MAX_FILTER_ITEMS = 5

/** Sidebar width */
export const SIDEBAR_WIDTH = 'w-64'

/** Excluded groups */
export const EXCLUDED_GROUPS = ['', 'auto']

/** Quota type values */
export const QUOTA_TYPE_VALUES = {
  TOKEN: 0,
  REQUEST: 1,
} as const

/** Token unit divisors */
export const TOKEN_UNIT_DIVISORS = {
  M: 1,
  K: 1000,
} as const

/** Default token unit for pricing display */
export const DEFAULT_TOKEN_UNIT: TokenUnit = 'M'

/** View mode options */
export const VIEW_MODES = {
  CARD: 'card',
  TABLE: 'table',
} as const

export type ViewMode = (typeof VIEW_MODES)[keyof typeof VIEW_MODES]

/** Default page size for pricing table */
export const DEFAULT_PRICING_PAGE_SIZE = 20

/**
 * Whether to offer the Standard / Recharge price-display switch in the pricing
 * toolbar. The switch only changes anything when the site's recharge rate
 * (Price) differs from the display exchange rate (USDExchangeRate); with both
 * equal it renders identical numbers. Hiding the control keeps every other
 * piece — the filter state, the props, and the conversion in
 * lib/dynamic-price.ts — so flipping this to true restores it.
 */
export const SHOW_RECHARGE_PRICE_MODE = false

/**
 * Page size for the pricing card grid.
 *
 * The grid lays out 1, 2 or 3 columns depending on the breakpoint (see
 * model-card-grid.tsx), so the page size is a multiple of all three and every
 * page ends on a whole row instead of a half-empty one. 12 also divides by 4,
 * so a fourth column could be added later without revisiting this. The table
 * keeps DEFAULT_PRICING_PAGE_SIZE and its own rows-per-page selector.
 */
export const DEFAULT_PRICING_CARD_PAGE_SIZE = 12
