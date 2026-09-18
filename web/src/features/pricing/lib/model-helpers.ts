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
import { EXCLUDED_GROUPS, FILTER_ALL, QUOTA_TYPE_VALUES } from '../constants'
import type { PricingModel } from '../types'

// ----------------------------------------------------------------------------
// Model Helper Utilities
// ----------------------------------------------------------------------------

/**
 * Get available groups for a model
 */
export function getAvailableGroups(
  model: PricingModel,
  usableGroup: Record<string, { desc: string; ratio: number }>
): string[] {
  const modelEnableGroups = Array.isArray(model.enable_groups)
    ? model.enable_groups
    : []

  return Object.keys(usableGroup)
    .filter((g) => !EXCLUDED_GROUPS.includes(g))
    .filter((g) => modelEnableGroups.includes(g))
}

/**
 * Read a configured group ratio while preserving valid zero ratios.
 */
export function getConfiguredGroupRatio(
  groupRatio: Record<string, number>,
  group: string
): number {
  const ratio = groupRatio[group]
  return typeof ratio === 'number' && Number.isFinite(ratio) ? ratio : 1
}

/**
 * Resolve the group ratio used by model square summary prices.
 *
 * When no specific group is selected, the model square shows the best price
 * available to the viewer. When a group filter is active, it shows that
 * group's price instead.
 */
export function getDisplayGroupRatio(
  model: PricingModel,
  selectedGroup?: string
): number {
  const modelEnableGroups = Array.isArray(model.enable_groups)
    ? model.enable_groups
    : []
  const groupRatio = model.group_ratio || {}

  if (
    selectedGroup &&
    selectedGroup !== FILTER_ALL &&
    modelEnableGroups.includes(selectedGroup)
  ) {
    return getConfiguredGroupRatio(groupRatio, selectedGroup)
  }

  if (modelEnableGroups.length === 0) {
    return 1
  }

  let minRatio = Number.POSITIVE_INFINITY

  for (const group of modelEnableGroups) {
    const ratio = groupRatio[group]
    if (
      typeof ratio === 'number' &&
      Number.isFinite(ratio) &&
      ratio < minRatio
    ) {
      minRatio = ratio
    }
  }

  return minRatio === Number.POSITIVE_INFINITY ? 1 : minRatio
}

/**
 * Resolve the billing group ratio for the current viewer.
 *
 * The pricing API already folds the logged-in user's per-user-group special
 * ratios into `groupRatio` and narrows `usableGroup` to what the viewer can
 * actually be billed under, so the viewer's group is the first entry of
 * `enable_groups ∩ usable_group` (single-group-per-user deployments make that
 * intersection exactly one group). Without an intersection (anonymous or no
 * usable group) we fall back to the best public price, matching the pricing
 * card behavior.
 */
export function getViewerGroupRatio(
  model: PricingModel,
  usableGroup: Record<string, { desc: string; ratio: number }>,
  groupRatio: Record<string, number>
): number {
  const viewerGroup = getAvailableGroups(model, usableGroup)[0]
  if (viewerGroup) {
    return getConfiguredGroupRatio(groupRatio, viewerGroup)
  }
  return getDisplayGroupRatio(model)
}

/**
 * Replace model placeholder in endpoint path
 */
export function replaceModelInPath(path: string, modelName: string): string {
  return path.replaceAll('{model}', modelName)
}

/**
 * Check if model is token-based pricing
 */
export function isTokenBasedModel(model: PricingModel): boolean {
  return model.quota_type === QUOTA_TYPE_VALUES.TOKEN
}
