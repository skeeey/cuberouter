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
import { describe, expect, test } from 'vitest'

import { getCurrencyLabel } from '@/lib/currency'
import dayjs from '@/lib/dayjs'
import { quotaUnitsToDollars } from '@/lib/format'

import type { UsageLog } from '../../data/schema'
import {
  buildCommonLogsExportSheet,
  collectCommonLogRows,
  pinLogsExportTimeRange,
  USAGE_LOGS_EXPORT_MAX_ROWS,
} from '../logs-export'

function cellValue(cell: unknown): unknown {
  if (cell && typeof cell === 'object' && 'value' in cell) {
    return (cell as { value?: unknown }).value
  }
  return cell
}

function translate(key: string): string {
  return key
}

function row(overrides: Partial<UsageLog> = {}): UsageLog {
  return {
    id: 1,
    user_id: 42,
    created_at: dayjs('2026-10-20T10:00:00').unix(),
    type: 2,
    content: '模型倍率 1.00，补全倍率 2.00',
    username: 'alice',
    token_name: 'key-1',
    model_name: 'deepseek-flash',
    quota: 800_000,
    prompt_tokens: 1_000,
    completion_tokens: 500,
    use_time: 1,
    is_stream: false,
    channel: 3,
    channel_name: 'prod',
    token_id: 9,
    group: 'default',
    ip: '',
    other: JSON.stringify({
      request_path: '/v1/chat/completions',
      cache_tokens: 300,
      cache_creation_tokens_5m: 100,
      cache_creation_tokens_1h: 50,
    }),
    request_id: 'req-1',
    upstream_request_id: 'up-1',
    ...overrides,
  }
}

describe('buildCommonLogsExportSheet', () => {
  test('exports the user columns for a text request', () => {
    const currency = getCurrencyLabel()
    const sheet = buildCommonLogsExportSheet([row()], false, translate)

    expect(sheet.rows[0].map(cellValue)).toEqual([
      'Time',
      'Log Type',
      'Usage Category',
      'Model',
      'Token',
      'Group',
      'Prompt Tokens',
      'Completion Tokens',
      'Cache Read Tokens',
      'Cache Write Tokens',
      'Usage Details',
      `Cost (${currency})`,
      'Request ID',
      'Upstream Request ID',
      'Task ID',
    ])
    expect(sheet.rows[1].map(cellValue)).toEqual([
      '2026-10-20 10:00:00',
      'Consume',
      'Text',
      'deepseek-flash',
      'key-1',
      'default',
      1_000,
      500,
      300,
      150,
      '模型倍率 1.00，补全倍率 2.00',
      quotaUnitsToDollars(800_000),
      'req-1',
      'up-1',
      '',
    ])
    expect(sheet.filename).toMatch(/^usage-logs-\d{4}-\d{2}-\d{2}-\d{6}\.xlsx$/)
  })

  test('adds the admin-only user, channel and upstream model columns', () => {
    const sheet = buildCommonLogsExportSheet(
      [
        row({
          other: JSON.stringify({
            request_path: '/v1/chat/completions',
            upstream_model_name: 'deepseek-upstream',
          }),
        }),
      ],
      true,
      translate
    )

    expect(sheet.rows[0].map(cellValue)).toEqual([
      'Time',
      'Log Type',
      'Usage Category',
      'User',
      'Channel',
      'Model',
      'Upstream Model',
      'Token',
      'Group',
      'Prompt Tokens',
      'Completion Tokens',
      'Cache Read Tokens',
      'Cache Write Tokens',
      'Usage Details',
      `Cost (${getCurrencyLabel()})`,
      'Request ID',
      'Upstream Request ID',
      'Task ID',
    ])
    expect(sheet.rows[1].slice(3, 8).map(cellValue)).toEqual([
      'alice',
      'prod #3',
      'deepseek-flash',
      'deepseek-upstream',
      'key-1',
    ])
  })

  test('classifies image and video rows and keeps their usage details', () => {
    const sheet = buildCommonLogsExportSheet(
      [
        row({
          model_name: 'wan2.7-image-a',
          content: '大小 1024x1024, 品质 standard, 生成数量 1',
          other: JSON.stringify({ request_path: '/v1/images/generations' }),
        }),
        row({
          model_name: 'seedance-2.0-a',
          content: '操作 text_to_video, 计算参数：seconds: 2.00, size: 0.25',
          prompt_tokens: 0,
          completion_tokens: 0,
          other: JSON.stringify({
            request_path: '/v1/video/generations',
            task_id: 'task_1',
          }),
        }),
      ],
      false,
      translate
    )

    expect(sheet.rows[1].slice(2, 3).map(cellValue)).toEqual(['Image'])
    expect(sheet.rows[1][10]).toBe('大小 1024x1024, 品质 standard, 生成数量 1')
    expect(sheet.rows[2].slice(2, 3).map(cellValue)).toEqual(['Video'])
    expect(sheet.rows[2][10]).toBe(
      '操作 text_to_video, 计算参数：seconds: 2.00, size: 0.25'
    )
    expect(sheet.rows[2][14]).toBe('task_1')
  })

  test('classifies task-plugin paths as video', () => {
    const sheet = buildCommonLogsExportSheet(
      [
        row({
          other: JSON.stringify({
            request_path: '/v1/tasks/astraflow-h3',
            usage_facts: { seconds: 4, resolution: '768P' },
          }),
        }),
      ],
      false,
      translate
    )

    expect(sheet.rows[1][2]).toBe('Video')
  })

  test('classifies embeddings and rerank as text usage', () => {
    const sheet = buildCommonLogsExportSheet(
      [
        row({ other: JSON.stringify({ request_path: '/v1/embeddings' }) }),
        row({ other: JSON.stringify({ request_path: '/v1/rerank' }) }),
      ],
      false,
      translate
    )

    expect(sheet.rows[1][2]).toBe('Text')
    expect(sheet.rows[2][2]).toBe('Text')
  })

  test('falls back to usage facts when content is empty', () => {
    const sheet = buildCommonLogsExportSheet(
      [
        row({
          content: '',
          other: JSON.stringify({
            request_path: '/v1/tasks/astraflow-h3',
            usage_facts: { seconds: 4, resolution: '768P' },
          }),
        }),
      ],
      false,
      translate
    )

    expect(sheet.rows[1][10]).toBe('seconds=4, resolution=768P')
  })

  test('shows audio characters for TTS and billing characters for speech', () => {
    const sheet = buildCommonLogsExportSheet(
      [
        row({
          model_name: 'moss-tts-1.5',
          content: '模型倍率 0.00，音频倍率 1.00',
          prompt_tokens: 4,
          completion_tokens: 33,
          other: JSON.stringify({
            request_path: '/v1/audio/speech',
            audio: true,
            text_input: 4,
            audio_output: 33,
          }),
        }),
        row({
          model_name: 'speech-2.8-hd',
          content: '',
          prompt_tokens: 4,
          completion_tokens: 0,
          other: JSON.stringify({ request_path: '/v1/audio/speech' }),
        }),
      ],
      false,
      translate
    )

    expect(sheet.rows[1][2]).toBe('Audio')
    expect(sheet.rows[1][10]).toBe('Audio Characters: 33')
    expect(sheet.rows[2][2]).toBe('Audio')
    expect(sheet.rows[2][10]).toBe('Billing Characters: 4')
  })

  test('sums split cache writes and falls back to the legacy field', () => {
    const sheet = buildCommonLogsExportSheet(
      [
        row({
          other: JSON.stringify({
            request_path: '/v1/messages',
            cache_creation_tokens_5m: 120,
            cache_creation_tokens_1h: 30,
          }),
        }),
        row({
          id: 2,
          other: JSON.stringify({
            request_path: '/v1/messages',
            cache_creation_tokens: 70,
          }),
        }),
      ],
      false,
      translate
    )

    expect(cellValue(sheet.rows[1][9])).toBe(150)
    expect(cellValue(sheet.rows[2][9])).toBe(70)
  })
})

describe('collectCommonLogRows', () => {
  function page(
    ids: number[],
    total: number,
    pageSize = 100
  ): { items: UsageLog[]; total: number; page_size: number } {
    return {
      items: ids.map((id) => row({ id })),
      total,
      page_size: pageSize,
    }
  }

  test('reads every page until the endpoint reports the set is complete', async () => {
    const fetchPage = async (pageNumber: number, pageSize: number) => {
      expect(pageSize).toBe(100)
      if (pageNumber === 1) {
        return page(
          Array.from({ length: 100 }, (_, index) => index + 1),
          250
        )
      }
      if (pageNumber === 2) {
        return page(
          Array.from({ length: 100 }, (_, index) => index + 101),
          250
        )
      }
      return page(
        Array.from({ length: 50 }, (_, index) => index + 201),
        250
      )
    }

    const collected = await collectCommonLogRows(fetchPage)

    expect(collected.items).toHaveLength(250)
    expect(collected.total).toBe(250)
    expect(collected.truncated).toBe(false)
  })

  test('stops at the row cap and reports a truncated set', async () => {
    const fetchPage = async (pageNumber: number) =>
      page(
        Array.from(
          { length: 100 },
          (_, index) => (pageNumber - 1) * 100 + index
        ),
        20_000
      )

    const collected = await collectCommonLogRows(fetchPage, { maxRows: 250 })

    expect(collected.items).toHaveLength(250)
    expect(collected.total).toBe(20_000)
    expect(collected.truncated).toBe(true)
  })

  test('does not ask for another page once the total is reached', async () => {
    let calls = 0
    const fetchPage = async () => {
      calls += 1
      return page(
        Array.from({ length: 100 }, (_, index) => index),
        100
      )
    }

    const collected = await collectCommonLogRows(fetchPage)

    expect(calls).toBe(1)
    expect(collected.items).toHaveLength(100)
    expect(collected.truncated).toBe(false)
  })

  test('defaults the cap to the 10,000-row export limit', () => {
    expect(USAGE_LOGS_EXPORT_MAX_ROWS).toBe(10_000)
  })

  test('treats a missing page payload as the end of the set', async () => {
    const collected = await collectCommonLogRows(async () => undefined)

    expect(collected.items).toEqual([])
    expect(collected.total).toBe(0)
    expect(collected.truncated).toBe(false)
  })
})

describe('pinLogsExportTimeRange', () => {
  const now = new Date(2026, 9, 29, 15, 30, 0).getTime()
  const startOfDay = new Date(2026, 9, 29, 0, 0, 0, 0).getTime()

  test('fills the default window and caps it at the export instant', () => {
    const pinned = pinLogsExportTimeRange({ model: 'deepseek-flash' }, now)

    expect(pinned).toEqual({
      model: 'deepseek-flash',
      startTime: startOfDay,
      endTime: now,
    })
  })

  test('keeps an explicit past window unchanged', () => {
    const pinned = pinLogsExportTimeRange(
      { startTime: startOfDay - 86_400_000, endTime: startOfDay },
      now
    )

    expect(pinned.startTime).toBe(startOfDay - 86_400_000)
    expect(pinned.endTime).toBe(startOfDay)
  })

  test('caps a future end at the export instant', () => {
    const pinned = pinLogsExportTimeRange(
      { startTime: startOfDay, endTime: now + 3_600_000 },
      now
    )

    expect(pinned.endTime).toBe(now)
  })

  test('fills a missing end with the export instant', () => {
    const pinned = pinLogsExportTimeRange({ startTime: startOfDay }, now)

    expect(pinned.startTime).toBe(startOfDay)
    expect(pinned.endTime).toBe(now)
  })
})
