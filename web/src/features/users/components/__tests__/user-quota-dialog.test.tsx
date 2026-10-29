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
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterAll, afterEach, beforeAll, describe, expect, test, vi } from 'vitest'

import { formatQuotaToCents } from '@/lib/currency'
import { parseQuotaFromDollars } from '@/lib/format'

import { adjustUserQuota } from '../../api'
import type { User } from '../../types'
import { UserQuotaDialog } from '../user-quota-dialog'

vi.mock('../../api', () => ({
  adjustUserQuota: vi.fn(),
}))

const adjustUserQuotaMock = vi.mocked(adjustUserQuota)
const originalGetAnimations = Object.getOwnPropertyDescriptor(
  HTMLElement.prototype,
  'getAnimations'
)

const user: User = {
  id: 7,
  username: 'topup-user',
  display_name: 'Topup User',
  email: 'topup@example.com',
  quota: 400000,
  used_quota: 200000,
  request_count: 1,
  group: 'default',
  status: 1,
  role: 1,
  cash_topup_total: 500000,
  gift_topup_total: 100000,
}

beforeAll(() => {
  Object.defineProperty(HTMLElement.prototype, 'getAnimations', {
    configurable: true,
    value: () => [],
  })
})

afterAll(() => {
  if (originalGetAnimations) {
    Object.defineProperty(
      HTMLElement.prototype,
      'getAnimations',
      originalGetAnimations
    )
    return
  }
  Reflect.deleteProperty(HTMLElement.prototype, 'getAnimations')
})

afterEach(() => {
  adjustUserQuotaMock.mockReset()
})

function renderDialog() {
  return render(
    <UserQuotaDialog
      open
      onOpenChange={() => undefined}
      user={user}
      currentQuota={user.quota}
      onSuccess={() => undefined}
    />
  )
}

describe('UserQuotaDialog', () => {
  test('shows the cash and gift breakdown under the current balance', () => {
    renderDialog()

    expect(
      screen.getByText(
        (_, element) =>
          element?.tagName === 'DIV' &&
          element.textContent ===
            `Cash ${formatQuotaToCents(500000)} / Gift ${formatQuotaToCents(100000)}`
      )
    ).toBeInTheDocument()
  })

  test('submits a cash top-up with the trimmed remark', async () => {
    adjustUserQuotaMock.mockResolvedValue({ success: true })
    const onSuccess = vi.fn()
    render(
      <UserQuotaDialog
        open
        onOpenChange={() => undefined}
        user={user}
        currentQuota={user.quota}
        onSuccess={onSuccess}
      />
    )

    fireEvent.change(screen.getByLabelText(/Recharge Amount/), {
      target: { value: '10' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Cash Account' }))
    fireEvent.change(screen.getByLabelText('Remark'), {
      target: { value: '  首充赠送  ' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Confirm' }))

    await waitFor(() =>
      expect(adjustUserQuotaMock).toHaveBeenCalledWith({
        id: user.id,
        action: 'add_quota',
        mode: 'add',
        value: parseQuotaFromDollars(10),
        account_type: 'cash',
        remark: '首充赠送',
      })
    )
    await waitFor(() => expect(onSuccess).toHaveBeenCalledTimes(1))
  })

  test('keeps confirm disabled until the amount is positive', () => {
    renderDialog()

    const confirm = screen.getByRole('button', { name: 'Confirm' })
    expect(confirm).toBeDisabled()

    fireEvent.change(screen.getByLabelText(/Recharge Amount/), {
      target: { value: '0' },
    })
    expect(confirm).toBeDisabled()

    fireEvent.change(screen.getByLabelText(/Recharge Amount/), {
      target: { value: '5' },
    })
    expect(confirm).toBeEnabled()
  })

  test('limits the amount input to two decimal places', () => {
    renderDialog()

    const input = screen.getByLabelText(/Recharge Amount/)
    expect(input).toHaveAttribute('step', '0.01')

    fireEvent.change(input, { target: { value: '10.12' } })
    expect(input).toHaveValue(10.12)

    fireEvent.change(input, { target: { value: '10.123' } })
    expect(input).toHaveValue(10.12)
  })
})
