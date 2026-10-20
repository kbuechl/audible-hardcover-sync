import { useState } from 'react'
import type { Status } from '../types'
import { syncNow } from '../api'
import { Button, Notice } from './ui'
import { CheckIcon } from './icons'
import { errorMessage } from './uiStyles'
import { timeAgo } from '../time'

interface AllCaughtUpProps {
  status: Status | null
  onSyncStarted: () => void
}

// AllCaughtUp is the Matches page when no book needs a manual match.
export default function AllCaughtUp({ status, onSyncStarted }: AllCaughtUpProps) {
  const [starting, setStarting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const ready = status !== null && status.audible.connected && status.hardcover.connected
  const running = status?.sync.running ?? false
  const last = status?.sync.last

  const start = async () => {
    setStarting(true)
    setError(null)
    try {
      await syncNow()
      onSyncStarted()
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setStarting(false)
    }
  }

  return (
    <div className="bg-panel border border-line rounded-xl px-6 py-12 text-center shadow-sm flex flex-col items-center gap-3">
      <span className="grid place-items-center size-14 rounded-full bg-pine-soft text-pine-text">
        <CheckIcon />
      </span>
      <h2 className="font-serif text-2xl font-semibold">All caught up</h2>
      <p className="text-sm text-muted max-w-sm">
        Nothing to resolve right now. Every book in your Audible library is matched on Hardcover.
      </p>
      {last && !last.error && (
        <p className="font-mono text-xs text-muted">
          Last sync {timeAgo(last.finishedAt)} · {last.synced} updated · {last.unchanged} unchanged
        </p>
      )}
      {ready && (
        <div className="mt-2">
          <Button kind="secondary" onClick={start} busy={starting || running}>
            {running ? 'Syncing…' : 'Sync now'}
          </Button>
        </div>
      )}
      {error && <Notice kind="error">{error}</Notice>}
    </div>
  )
}
