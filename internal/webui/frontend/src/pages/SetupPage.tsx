import { useState, type FormEvent } from 'react'
import type { Status } from '../types'
import { finishAudibleLogin, logoutAudible, resetSync, setHardcoverToken, startAudibleLogin, syncNow } from '../api'
import { Badge, Button, Card, Notice } from '../components/ui'
import { errorMessage, inputClass } from '../components/uiStyles'
import { BookIcon, HeadphonesIcon, SyncIcon } from '../components/icons'
import { timeAgo } from '../time'
import PreferencesCard from '../components/PreferencesCard'

interface SetupPageProps {
  status: Status | null
  onChanged: () => void
}

export default function SetupPage({ status, onChanged }: SetupPageProps) {
  if (status === null) return <p className="text-muted text-sm">Loading…</p>
  return (
    <div className="flex flex-col gap-6">
      <AudibleCard status={status.audible} onChanged={onChanged} />
      <HardcoverCard status={status.hardcover} onChanged={onChanged} />
      <SyncCard status={status} onChanged={onChanged} />
      <PreferencesCard />
    </div>
  )
}

function AudibleCard({ status, onChanged }: { status: Status['audible']; onChanged: () => void }) {
  const [signInUrl, setSignInUrl] = useState<string | null>(null)
  const [redirect, setRedirect] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const run = async (action: () => Promise<void>) => {
    setBusy(true)
    setError(null)
    try {
      await action()
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  const start = () =>
    run(async () => {
      const url = await startAudibleLogin()
      setSignInUrl(url)
      window.open(url, '_blank', 'noopener')
    })

  const finish = (e: FormEvent) => {
    e.preventDefault()
    run(async () => {
      await finishAudibleLogin(redirect)
      setSignInUrl(null)
      setRedirect('')
      onChanged()
    })
  }

  const logout = () => {
    if (!window.confirm('Log out of Audible? This removes the device from your Amazon account and syncing stops until you connect again.')) return
    run(async () => {
      await logoutAudible()
      onChanged()
    })
  }

  return (
    <Card title="Audible" icon={<HeadphonesIcon />} badge={<Badge ok={status.connected}>{status.connected ? 'Connected' : 'Not connected'}</Badge>}>
      {status.error && <div className="mb-4"><Notice kind="error">{status.error}</Notice></div>}
      {signInUrl === null ? (
        <div className="flex flex-wrap items-center gap-3">
          <p className="text-sm text-muted flex-1 min-w-60">
            {status.connected
              ? 'Your Audible library is synced with the login saved on this server.'
              : 'Sign in to Amazon to let this tool read your Audible library and listening progress.'}
          </p>
          <Button kind={status.connected ? 'secondary' : 'primary'} onClick={start} busy={busy && signInUrl === null}>
            {status.connected ? 'Reconnect' : 'Connect Audible'}
          </Button>
          {status.connected && (
            <Button kind="danger" onClick={logout} disabled={busy}>
              Log out
            </Button>
          )}
        </div>
      ) : (
        <form onSubmit={finish} className="flex flex-col gap-3">
          <ol className="text-sm list-decimal pl-5 space-y-1">
            <li>
              Sign in on the Amazon page that just opened.{' '}
              <a href={signInUrl} target="_blank" rel="noreferrer" className="text-pine-text underline decoration-dotted">
                Open it again ↗
              </a>
            </li>
            <li>After signing in you'll land on a page that may look like an error. That's expected.</li>
            <li>Copy that page's full address from the address bar and paste it here.</li>
          </ol>
          <input
            className={inputClass}
            value={redirect}
            onChange={(e) => setRedirect(e.target.value)}
            placeholder="https://www.amazon.com/ap/maplanding?..."
            aria-label="Address after signing in"
            autoComplete="off"
          />
          <div className="flex gap-2">
            <Button type="submit" busy={busy} disabled={redirect.trim() === ''}>
              {busy ? 'Connecting…' : 'Finish connecting'}
            </Button>
            <Button kind="secondary" onClick={() => setSignInUrl(null)} disabled={busy}>
              Cancel
            </Button>
          </div>
        </form>
      )}
      {error && <div className="mt-4"><Notice kind="error">{error}</Notice></div>}
    </Card>
  )
}

function HardcoverCard({ status, onChanged }: { status: Status['hardcover']; onChanged: () => void }) {
  const [token, setToken] = useState('')
  const [editing, setEditing] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const save = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      await setHardcoverToken(token)
      setToken('')
      setEditing(false)
      onChanged()
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  const showForm = !status.fromEnv && (editing || !status.connected)
  return (
    <Card
      title="Hardcover"
      icon={<BookIcon />}
      badge={<Badge ok={status.connected}>{status.connected ? `@${status.username}` : 'Not connected'}</Badge>}
    >
      {status.error && <div className="mb-4"><Notice kind="error">{status.error}</Notice></div>}
      {status.fromEnv ? (
        <p className="text-sm text-muted">
          The API token comes from the <code className="font-mono text-xs">HARDCOVER_API_TOKEN</code> environment variable.
          Change it there.
        </p>
      ) : showForm ? (
        <form onSubmit={save} className="flex flex-col gap-3">
          <p className="text-sm text-muted">
            Copy your API token from{' '}
            <a href="https://hardcover.app/account/api" target="_blank" rel="noreferrer" className="text-pine-text underline decoration-dotted">
              hardcover.app → Settings → API ↗
            </a>
            . It's checked with Hardcover, then saved on this server. It's never shown again.
          </p>
          <input
            type="password"
            className={inputClass}
            value={token}
            onChange={(e) => setToken(e.target.value)}
            placeholder="Paste API token"
            aria-label="Hardcover API token"
            autoComplete="off"
          />
          <div className="flex gap-2">
            <Button type="submit" busy={busy} disabled={token.trim() === ''}>
              {busy ? 'Checking…' : 'Save token'}
            </Button>
            {status.connected && (
              <Button kind="secondary" onClick={() => setEditing(false)} disabled={busy}>
                Cancel
              </Button>
            )}
          </div>
        </form>
      ) : (
        <div className="flex flex-wrap items-center gap-3">
          <p className="text-sm text-muted flex-1 min-w-60">Books are written to this Hardcover account.</p>
          <Button kind="secondary" onClick={() => setEditing(true)}>
            Change token
          </Button>
        </div>
      )}
      {error && <div className="mt-4"><Notice kind="error">{error}</Notice></div>}
    </Card>
  )
}

function SyncCard({ status, onChanged }: { status: Status; onChanged: () => void }) {
  const [error, setError] = useState<string | null>(null)
  const [message, setMessage] = useState<string | null>(null)
  const [starting, setStarting] = useState(false)
  const ready = status.audible.connected && status.hardcover.connected
  const last = status.sync.last

  const act = async (action: () => Promise<void>, done: string) => {
    setError(null)
    setMessage(null)
    try {
      await action()
      setMessage(done)
      onChanged()
    } catch (err) {
      setError(errorMessage(err))
    }
  }

  const reset = () => {
    if (!window.confirm('Reset sync history? Your Hardcover library is not changed. The next sync re-checks every book.')) return
    act(resetSync, 'Sync history cleared. The next sync re-checks your whole library.')
  }

  return (
    <Card title="Sync" icon={<SyncIcon />} badge={status.sync.running ? <Badge ok>Running…</Badge> : undefined}>
      {last ? (
        <dl className="grid grid-cols-2 sm:grid-cols-5 gap-2 mb-4 font-mono text-sm">
          <Stat label="Last run" value={timeAgo(last.finishedAt)} title={new Date(last.finishedAt).toLocaleString()} wide />
          <Stat label="Synced" value={last.synced} />
          <Stat label="Unmatched" value={last.unmatched} />
          <Stat label="Errors" value={last.errored} />
        </dl>
      ) : (
        <p className="text-sm text-muted mb-4">No sync has run yet.</p>
      )}
      {last?.error && <div className="mb-4"><Notice kind="error">Last sync failed: {last.error}</Notice></div>}
      {!ready && <div className="mb-4"><Notice kind="info">Connect Audible and Hardcover above to start syncing.</Notice></div>}
      <div className="flex flex-wrap gap-2">
        <Button
          onClick={() => {
            setStarting(true)
            act(async () => {
              await syncNow()
              // The sync starts in the background; give it a moment to show as running.
              await new Promise((r) => setTimeout(r, 500))
            }, 'Sync started.').finally(() => setStarting(false))
          }}
          busy={starting || status.sync.running}
          disabled={!ready}
        >
          {status.sync.running ? 'Syncing…' : 'Sync now'}
        </Button>
        <Button kind="danger" onClick={reset} disabled={status.sync.running}>
          Reset sync history
        </Button>
      </div>
      {message && <div className="mt-4"><Notice kind="ok">{message}</Notice></div>}
      {error && <div className="mt-4"><Notice kind="error">{error}</Notice></div>}
    </Card>
  )
}

function Stat({ label, value, title, wide }: { label: string; value: string | number; title?: string; wide?: boolean }) {
  return (
    <div className={`rounded-lg bg-paper px-3 py-2 ${wide ? 'col-span-2' : ''}`}>
      <dt className="text-[0.68rem] uppercase tracking-wide text-muted">{label}</dt>
      <dd className="text-base" title={title}>{value}</dd>
    </div>
  )
}
