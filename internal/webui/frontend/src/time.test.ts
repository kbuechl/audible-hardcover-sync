import { describe, it, expect } from 'vitest'
import { timeAgo } from './time'

describe('timeAgo', () => {
  const now = Date.parse('2026-10-20T12:00:00Z')
  it.each([
    ['2026-10-20T11:59:40Z', 'just now'],
    ['2026-10-20T11:48:00Z', '12 min ago'],
    ['2026-10-20T11:00:00Z', '1 hour ago'],
    ['2026-10-20T07:00:00Z', '5 hours ago'],
    ['2026-10-18T12:00:00Z', '2 days ago'],
    ['2026-10-20T12:05:00Z', 'just now'],
  ])('%s is %s', (iso, want) => {
    expect(timeAgo(iso, now)).toBe(want)
  })
})
