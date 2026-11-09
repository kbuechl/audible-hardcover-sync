import { useEffect, useState, type FormEvent } from 'react'
import type { NotifyTarget, Preferences } from '../types'
import { addNotifyURL, fetchNotifyTargets, fetchPreferences, savePreferences, removeNotifyTarget, testNotifyTarget, testNotifyURL } from '../api'
import { Button, Card, Notice } from '../components/ui'
import { errorMessage, inputClass } from '../components/uiStyles'
import { BellIcon } from '../components/icons'

const reviewNotifyOptions: { value: Preferences['reviewNotify']; label: string }[] = [
  { value: 'after_sync', label: 'After each sync' },
  { value: 'daily', label: 'Once a day' },
  { value: 'weekly', label: 'Once a week' },
  { value: 'off', label: 'Off' },
]

type Result = { kind: 'ok' | 'error'; text: string }

export default function NotificationsPage() {
  const [targets, setTargets] = useState<NotifyTarget[] | null>(null)
  const [url, setUrl] = useState('')
  const [busy, setBusy] = useState(false)
  const [formResult, setFormResult] = useState<Result | null>(null)
  const [rowResults, setRowResults] = useState<Record<number, Result>>({})

  const [prefs, setPrefs] = useState<Preferences | null>(null)
  const [reviewResult, setReviewResult] = useState<Result | null>(null)

  const reload = () => fetchNotifyTargets().then(setTargets)

  useEffect(() => {
    reload().catch((err) => setFormResult({ kind: 'error', text: errorMessage(err) }))
  }, [])

  useEffect(() => {
    fetchPreferences()
      .then(setPrefs)
      .catch((err) => setReviewResult({ kind: 'error', text: errorMessage(err) }))
  }, [])

  // Saves the full preferences object with one field changed, so nothing else is clobbered.
  const changeReviewNotify = async (reviewNotify: Preferences['reviewNotify']) => {
    if (prefs === null) return
    setReviewResult(null)
    try {
      setPrefs(await savePreferences({ ...prefs, reviewNotify }))
      setReviewResult({ kind: 'ok', text: 'Saved.' })
    } catch (err) {
      setReviewResult({ kind: 'error', text: errorMessage(err) })
    }
  }

  const run = async (action: () => Promise<void>, ok: string) => {
    setBusy(true)
    setFormResult(null)
    try {
      await action()
      setFormResult({ kind: 'ok', text: ok })
    } catch (err) {
      setFormResult({ kind: 'error', text: errorMessage(err) })
    } finally {
      setBusy(false)
    }
  }

  const add = (e: FormEvent) => {
    e.preventDefault()
    run(async () => {
      await addNotifyURL(url)
      setUrl('')
      setRowResults({})
      await reload()
    }, 'Saved.')
  }

  const testRow = async (id: number) => {
    setRowResults((r) => ({ ...r, [id]: { kind: 'ok', text: 'Sending…' } }))
    try {
      await testNotifyTarget(id)
      setRowResults((r) => ({ ...r, [id]: { kind: 'ok', text: 'Test sent.' } }))
    } catch (err) {
      setRowResults((r) => ({ ...r, [id]: { kind: 'error', text: errorMessage(err) } }))
    }
  }

  const remove = async (id: number) => {
    try {
      await removeNotifyTarget(id)
      setRowResults({})
      await reload()
    } catch (err) {
      setRowResults((r) => ({ ...r, [id]: { kind: 'error', text: errorMessage(err) } }))
    }
  }

  return (
    <div className="flex flex-col gap-6">
      <Card title="Notification services" icon={<BellIcon />}>
        <p className="text-sm text-muted mb-4">
          Get a message when books need matching by hand. Saved addresses are shown with their secrets hidden.
        </p>
        {targets === null && <p className="text-sm text-muted">Loading…</p>}
        {targets?.length === 0 && (
          <p className="text-sm text-muted border border-dashed border-line rounded-lg p-4 text-center">
            No notification services yet. Add one below.
          </p>
        )}
        <ul className="flex flex-col gap-2">
          {targets?.map((t) => (
            <li key={t.id} className="border border-line rounded-lg p-3 bg-paper">
              <div className="flex flex-wrap items-center gap-3">
                <code className="font-mono text-sm basis-full sm:basis-0 flex-1 min-w-0 break-all">{t.masked}</code>
                <Button kind="secondary" onClick={() => testRow(t.id)}>
                  Send test
                </Button>
                <Button kind="danger" onClick={() => remove(t.id)}>
                  Remove
                </Button>
              </div>
              {rowResults[t.id] && (
                <div className="mt-2">
                  <Notice kind={rowResults[t.id].kind}>{rowResults[t.id].text}</Notice>
                </div>
              )}
            </li>
          ))}
        </ul>
      </Card>

      <Card title="Books waiting for review" icon={<BellIcon />}>
        <div className="flex flex-col gap-1.5">
          <label htmlFor="reviewNotify" className="text-sm font-semibold">
            Review message
          </label>
          <select
            id="reviewNotify"
            aria-describedby="reviewNotify-hint"
            className={inputClass}
            value={prefs?.reviewNotify ?? ''}
            disabled={prefs === null}
            onChange={(e) => changeReviewNotify(e.target.value as Preferences['reviewNotify'])}
          >
            {prefs === null && <option value="" hidden />}
            {reviewNotifyOptions.map((o) => (
              <option key={o.value} value={o.value}>
                {o.label}
              </option>
            ))}
          </select>
          <span id="reviewNotify-hint" className="text-xs text-muted">
            How often to get a message about books waiting on the Review page.
          </span>
        </div>
        {reviewResult && <div className="mt-3"><Notice kind={reviewResult.kind}>{reviewResult.text}</Notice></div>}
      </Card>

      <Card title="Add a service">
        <form onSubmit={add} className="flex flex-col gap-3">
          <p className="text-sm text-muted">
            Paste a{' '}
            <a
              href="https://containrrr.dev/shoutrrr/latest/services/overview/"
              target="_blank"
              rel="noreferrer"
              className="text-pine-text underline decoration-dotted"
            >
              shoutrrr URL ↗
            </a>{' '}
            for Discord, Slack, Telegram, ntfy, Pushover, email and more. For example{' '}
            <code className="font-mono text-xs">discord://token@webhookid</code> or{' '}
            <code className="font-mono text-xs">ntfy://ntfy.sh/my-topic</code>.
          </p>
          <input
            className={inputClass}
            value={url}
            onChange={(e) => setUrl(e.target.value)}
            placeholder="service://…"
            aria-label="Notification URL"
            autoComplete="off"
          />
          <div className="flex gap-2">
            <Button kind="secondary" onClick={() => run(() => testNotifyURL(url), 'Test sent. Check that it arrived.')} disabled={busy || url.trim() === ''}>
              Send test
            </Button>
            <Button type="submit" disabled={busy || url.trim() === ''}>
              Save
            </Button>
          </div>
          {formResult && <Notice kind={formResult.kind}>{formResult.text}</Notice>}
        </form>
      </Card>
    </div>
  )
}
