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
import { render, screen } from '@testing-library/react'
import { describe, expect, test, vi } from 'vitest'

import type { OrganizationLogRow } from '../../types'
import { OrganizationLogDetailsDialog } from '../organization-log-details-dialog'

// @lobehub/icons transitively imports @emoji-mart JSON assets that vitest's
// externalized ESM loader rejects. Icons are irrelevant to the fields under
// test, so the icon loader boundary is stubbed.
vi.mock('@/lib/lobe-icon', () => ({
  getLobeIcon: () => null,
}))

function logRow(): OrganizationLogRow {
  return {
    id: 1,
    user_id: 42,
    created_at: 1_700_000_000,
    type: 2,
    token_name: 'key-1',
    model_name: 'gpt-4o',
    quota: 1_000,
    prompt_tokens: 10,
    completion_tokens: 5,
    use_time: 1,
    is_stream: false,
    group: 'vip-group',
    request_id: 'request-1',
  }
}

describe('organization log details dialog', () => {
  test('does not list the group', () => {
    render(
      <OrganizationLogDetailsDialog
        row={logRow()}
        showResponsible
        onOpenChange={() => undefined}
      />
    )

    expect(screen.getByText('request-1')).toBeInTheDocument()
    expect(screen.queryByText('Group')).not.toBeInTheDocument()
    expect(screen.queryByText('vip-group')).not.toBeInTheDocument()
  })
})
