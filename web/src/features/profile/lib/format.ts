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
import type { NotifyType, UserProfile, UserSettings } from '../types'

// ============================================================================
// Profile Formatting Utilities
// ============================================================================

/**
 * Every notify_type the backend persists.
 *
 * Deliberately wider than the notification selector's NOTIFICATION_METHODS
 * (features/profile/constants.ts), which only controls what the UI offers:
 * a profile that already stores webhook / bark / gotify keeps that value, so
 * loading and re-saving the settings form cannot silently switch the user's
 * delivery channel to email.
 */
const PERSISTED_NOTIFY_TYPES = new Set<NotifyType>([
  'email',
  'webhook',
  'bark',
  'gotify',
])

/**
 * Parse user settings from JSON string
 */
export function parseUserSettings(settingsJson?: string): UserSettings {
  if (!settingsJson) return {}

  try {
    return JSON.parse(settingsJson) as UserSettings
  } catch {
    return {}
  }
}

/**
 * Normalize a stored notify_type: keep anything the backend can persist and
 * fall back to email only for values that are missing or unknown.
 */
export function normalizeNotifyType(value: unknown): NotifyType {
  return typeof value === 'string' &&
    PERSISTED_NOTIFY_TYPES.has(value as NotifyType)
    ? (value as NotifyType)
    : 'email'
}

/**
 * Get display name or fallback to username
 */
export function getDisplayName(user?: UserProfile): string {
  if (!user) return ''
  return user.display_name || user.username
}

/**
 * Get user initials for avatar
 */
export function getUserInitials(user?: UserProfile): string {
  if (!user) return '?'
  const name = getDisplayName(user)
  if (!name) return '?'

  const parts = name.trim().split(/\s+/)
  if (parts.length >= 2) {
    return `${parts[0][0]}${parts[1][0]}`.toUpperCase()
  }
  return name.slice(0, 2).toUpperCase()
}
