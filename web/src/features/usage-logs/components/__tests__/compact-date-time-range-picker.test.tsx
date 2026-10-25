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
import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, test, vi } from 'vitest'

import dayjs from '@/lib/dayjs'

import { CompactDateTimeRangePicker } from '../compact-date-time-range-picker'

describe('compact date time range picker', () => {
  test('day precision uses date-only inputs and returns whole-day boundaries', async () => {
    const onChange = vi.fn()
    render(
      <CompactDateTimeRangePicker
        start={dayjs('2026-10-01').toDate()}
        end={dayjs('2026-10-05').toDate()}
        onChange={onChange}
        precision='day'
      />
    )

    expect(
      screen.getByRole('button', { name: '2026-10-01 ~ 2026-10-05' })
    ).toBeInTheDocument()

    await userEvent.click(
      screen.getByRole('button', { name: '2026-10-01 ~ 2026-10-05' })
    )

    const dateInputs =
      document.querySelectorAll<HTMLInputElement>('input[type="date"]')
    expect(dateInputs).toHaveLength(2)
    expect(
      document.querySelector('input[type="datetime-local"]')
    ).not.toBeInTheDocument()

    fireEvent.change(dateInputs[0], { target: { value: '2026-10-02' } })
    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }))

    expect(onChange).toHaveBeenCalledWith({
      start: dayjs('2026-10-02').startOf('day').toDate(),
      end: dayjs('2026-10-05').endOf('day').toDate(),
    })
  })

  test('minute precision stays the default for existing callers', async () => {
    render(
      <CompactDateTimeRangePicker
        start={dayjs('2026-10-01T08:30').toDate()}
        end={dayjs('2026-10-05T18:45').toDate()}
        onChange={() => undefined}
      />
    )

    await userEvent.click(
      screen.getByRole('button', {
        name: '2026-10-01 08:30 ~ 2026-10-05 18:45',
      })
    )

    expect(
      document.querySelectorAll('input[type="datetime-local"]')
    ).toHaveLength(2)
    expect(document.querySelector('input[type="date"]')).not.toBeInTheDocument()
  })
})
