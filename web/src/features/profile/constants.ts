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
// ============================================================================
// Profile Constants
// ============================================================================

/**
 * Default quota warning threshold (500,000 = $1)
 */
export const DEFAULT_QUOTA_WARNING_THRESHOLD = 500000

/**
 * Whether to expose the "Accept Unpriced Models" preference in the UI.
 * The backend field (`accept_unset_model_ratio_model`) still works; this flag
 * only gates the toggle on the profile page so it can be re-enabled later.
 */
export const SHOW_ACCEPT_UNPRICED_MODELS = false

/**
 * Whether to show the Passkey card on the profile page. The WebAuthn backend
 * endpoints still work; this flag only hides the card so Passkey management
 * can be re-enabled later by flipping it to true.
 */
export const SHOW_PASSKEY_CARD = false

/**
 * Whether to show the "Sidebar Modules" personalization card on the profile
 * page. The backend permission (`permissions.sidebar_settings`) still works;
 * this flag only hides the card so it can be re-enabled later.
 */
export const SHOW_SIDEBAR_MODULES_CARD = false

/**
 * Notification methods
 *
 * NOTE: only Email is offered to users for now. The webhook / bark / gotify
 * entries below are intentionally commented out instead of deleted so they
 * can be re-enabled later; the matching conditional forms in
 * notification-tab.tsx are kept as dead code on purpose.
 */
export const NOTIFICATION_METHODS = [
  { value: 'email' as const, label: 'Email' },
  // { value: 'webhook' as const, label: 'Webhook' },
  // { value: 'bark' as const, label: 'Bark' },
  // { value: 'gotify' as const, label: 'Gotify' },
] as const
