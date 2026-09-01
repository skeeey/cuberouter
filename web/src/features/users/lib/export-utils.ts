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
export interface ExportUsersPayload {
  ids?: number[]
  keyword?: string
  group?: string
}

/**
 * Decide the admin export payload: selected ids win when present, otherwise
 * the current table filter (keyword/group) is sent. Mirrors the backend rule
 * that ids take precedence over the filter; empty fields are omitted so an
 * empty payload means "export all users".
 */
export function buildExportPayload(
  selectedIds: number[],
  filter: { keyword: string; group: string }
): ExportUsersPayload {
  if (selectedIds.length > 0) {
    return { ids: selectedIds }
  }
  const payload: ExportUsersPayload = {}
  if (filter.keyword) {
    payload.keyword = filter.keyword
  }
  if (filter.group) {
    payload.group = filter.group
  }
  return payload
}
