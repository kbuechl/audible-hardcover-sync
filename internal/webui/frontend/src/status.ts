import type { Status } from './types'
import { timeAgo } from './time'

export type Tone = 'ok' | 'warn' | 'busy' | 'idle'

// describeStatus summarizes the app's health for the header pill.
export function describeStatus(status: Status): { tone: Tone; text: string; short: string; href: string } {
  if (!status.audible.connected || !status.hardcover.connected) {
    return { tone: 'warn', text: 'Setup needed', short: 'Setup needed', href: '#setup' }
  }
  if (status.sync.running) return { tone: 'busy', text: 'Syncing…', short: 'Syncing…', href: '#setup' }
  const last = status.sync.last
  if (!last) return { tone: 'idle', text: 'Not synced yet', short: 'Not synced yet', href: '#setup' }
  if (last.error) return { tone: 'warn', text: 'Last sync failed', short: 'Last sync failed', href: '#setup' }
  return { tone: 'ok', text: `Synced ${timeAgo(last.finishedAt)}`, short: timeAgo(last.finishedAt), href: '#setup' }
}

