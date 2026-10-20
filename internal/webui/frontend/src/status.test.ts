import { describe, it, expect } from 'vitest'
import { describeStatus } from './status'
import type { Status } from './types'

const base: Status = {
  audible: { connected: true },
  hardcover: { connected: true, fromEnv: false },
  sync: { running: false },
}

describe('describeStatus', () => {
  it('asks for setup when something is not connected', () => {
    expect(describeStatus({ ...base, audible: { connected: false } }).text).toBe('Setup needed')
  })
  it('shows a running sync', () => {
    expect(describeStatus({ ...base, sync: { running: true } }).text).toBe('Syncing…')
  })
  it('shows a failed sync', () => {
    const last = { finishedAt: new Date().toISOString(), synced: 0, unchanged: 0, unmatched: 0, errored: 0, error: 'boom' }
    expect(describeStatus({ ...base, sync: { running: false, last } }).tone).toBe('warn')
  })
  it('shows when it last synced', () => {
    const last = { finishedAt: new Date().toISOString(), synced: 1, unchanged: 2, unmatched: 0, errored: 0 }
    expect(describeStatus({ ...base, sync: { running: false, last } }).text).toBe('Synced just now')
  })
})
