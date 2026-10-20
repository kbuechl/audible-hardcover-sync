import { useCallback, useEffect, useState } from 'react'
import type { Status } from './types'
import { fetchStatus } from './api'
import MatchesPage from './pages/MatchesPage'
import SetupPage from './pages/SetupPage'
import NotificationsPage from './pages/NotificationsPage'
import StatusPill from './components/StatusPill'
import ThemeToggle from './components/ThemeToggle'

const tabs = [
  { id: 'matches', label: 'Matches' },
  { id: 'setup', label: 'Setup' },
  { id: 'notifications', label: 'Notifications' },
] as const

type Tab = (typeof tabs)[number]['id']

function tabFromHash(): Tab | null {
  const id = window.location.hash.replace('#', '')
  return tabs.some((t) => t.id === id) ? (id as Tab) : null
}

const POLL_MS = 3000
// Refresh now and then so "Synced 5 min ago" stays current and scheduled
// syncs show up.
const IDLE_POLL_MS = 60_000

export default function App() {
  const [status, setStatus] = useState<Status | null>(null)
  const [tab, setTab] = useState<Tab | null>(tabFromHash)

  const refresh = useCallback(() => {
    fetchStatus()
      .then(setStatus)
      .catch(() => setStatus(null))
  }, [])

  useEffect(refresh, [refresh])

  // Follow a running sync until it finishes.
  useEffect(() => {
    const timer = window.setInterval(refresh, status?.sync.running ? POLL_MS : IDLE_POLL_MS)
    return () => window.clearInterval(timer)
  }, [status?.sync.running, refresh])

  useEffect(() => {
    const onHash = () => setTab(tabFromHash())
    window.addEventListener('hashchange', onHash)
    return () => window.removeEventListener('hashchange', onHash)
  }, [])

  const needsSetup = status !== null && (!status.audible.connected || !status.hardcover.connected)
  // Until the user picks a tab, open on Setup when something isn't connected.
  // Decided once, on the first status load, so finishing setup doesn't jump
  // away from the Setup page.
  const [startTab, setStartTab] = useState<Tab | null>(null)
  if (startTab === null && status !== null) setStartTab(needsSetup ? 'setup' : 'matches')
  const active: Tab = tab ?? startTab ?? 'matches'

  return (
    <div className="max-w-5xl mx-auto px-4 sm:px-6 py-6 sm:py-8">
      <header className="flex items-center justify-between gap-3 mb-3">
        <a href="#matches" className="flex items-center gap-2.5 min-w-0">
          <img src="/favicon.svg" alt="" className="size-6 sm:size-7 shrink-0" />
          <span className="font-serif font-semibold text-base sm:text-lg truncate">
            audible <span className="text-muted">→</span> hardcover
          </span>
        </a>
        <StatusPill status={status} />
      </header>
      <nav className="flex items-center gap-1 mb-6 sm:mb-8 border-b border-line">
        {tabs.map((t) => (
          <a
            key={t.id}
            href={`#${t.id}`}
            aria-current={active === t.id ? 'page' : undefined}
            className={`shrink-0 text-sm px-3 py-2.5 -mb-px border-b-2 transition-colors ${
              active === t.id ? 'border-pine text-ink font-semibold' : 'border-transparent text-muted hover:text-ink'
            }`}
          >
            {t.label}
            {t.id === 'setup' && needsSetup && <span className="ml-1.5 text-burgundy">•</span>}
          </a>
        ))}
        <div className="ml-auto pb-1.5">
          <ThemeToggle />
        </div>
      </nav>

      <main key={active} className="fade-in">
        {active === 'matches' && <MatchesPage status={status} onSyncStarted={refresh} />}
        {active === 'setup' && <SetupPage status={status} onChanged={refresh} />}
        {active === 'notifications' && <NotificationsPage />}
      </main>
    </div>
  )
}
