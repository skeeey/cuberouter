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
import { render, screen } from '@testing-library/react'
import { describe, expect, test } from 'vitest'

import { ModelPerfBadge } from '../components/model-perf-badge'

describe('ModelPerfBadge', () => {
  test('renders placeholder dashes for every metric when perf data is absent', () => {
    const { container } = render(<ModelPerfBadge perf={undefined} />)

    expect(screen.getByText('Latency short')).toBeInTheDocument()
    expect(screen.getByText('Throughput short')).toBeInTheDocument()
    expect(screen.getByText('Status short')).toBeInTheDocument()
    // latency, throughput and status each fall back to a dash
    expect(screen.getAllByText('—')).toHaveLength(3)
    // status bars are not rendered without any success-rate data
    expect(
      container.querySelectorAll('[class*="bg-muted-foreground/"]')
    ).toHaveLength(0)
    // no misleading NaN percentage in the success-rate tooltip
    expect(screen.getByTitle('Success rate')).toBeInTheDocument()
  })

  test('right-aligns placeholder dashes so they line up with their column labels', () => {
    const { container } = render(<ModelPerfBadge perf={undefined} />)
    const dashes = screen.getAllByText('—')

    // 三列共用的对齐契约是右对齐,占位 dash 必须继承它而不是自己改对齐
    expect(container.firstElementChild?.className).toContain('text-right')
    expect(dashes[0].className).not.toMatch(/text-(center|left)/)
    expect(dashes[1].className).not.toMatch(/text-(center|left)/)
    // 状态列的占位 dash 与状态条在同一侧
    expect(dashes[2].parentElement?.className).toContain('justify-end')
  })

  test('renders metric values when perf data exists', () => {
    render(
      <ModelPerfBadge
        perf={{
          avg_latency_ms: 820,
          success_rate: 99.2,
          avg_tps: 45,
          recent_success_rates: [98.1, 99.5, 99.2],
        }}
      />
    )

    expect(screen.getByText('820ms')).toBeInTheDocument()
    expect(screen.getByText('45t')).toBeInTheDocument()
    expect(screen.getByTitle('Success rate: 99.2%')).toBeInTheDocument()
  })
})
