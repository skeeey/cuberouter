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
import { describe, expect, test } from 'vitest'

import { ProfileHeader } from '../components/profile-header'
import type { UserProfile } from '../types'

const profile: UserProfile = {
  id: 7,
  username: 'alice',
  display_name: 'Alice',
  role: 1,
  email: 'alice@example.com',
  group: 'default',
  quota: 1_000_000,
  used_quota: 2_000_000,
  request_count: 36,
  status: 1,
  aff_count: 0,
  aff_quota: 0,
  aff_history_quota: 0,
  created_time: 1_700_000_000,
}

describe('profile header', () => {
  test('shows the identity details without the balance, usage and request statistics', () => {
    render(<ProfileHeader profile={profile} loading={false} />)

    expect(screen.getByText('Alice')).toBeInTheDocument()
    expect(screen.getByText('@alice')).toBeInTheDocument()

    expect(screen.queryByText('Current Balance')).not.toBeInTheDocument()
    expect(screen.queryByText('Total Usage')).not.toBeInTheDocument()
    expect(screen.queryByText('API Requests')).not.toBeInTheDocument()
  })
})
