import { useEffect, useState, type FormEvent } from 'react'
import type { Preferences } from '../types'
import { fetchPreferences, savePreferences } from '../api'
import { Button, Card, Notice } from './ui'
import { SlidersIcon } from './icons'
import { errorMessage, inputClass } from './uiStyles'

// Values match Go's time.Duration strings so the saved one is selected.
const intervals = [
  { value: '15m0s', label: 'Every 15 minutes' },
  { value: '30m0s', label: 'Every 30 minutes' },
  { value: '1h0m0s', label: 'Every hour' },
  { value: '6h0m0s', label: 'Every 6 hours' },
  { value: '12h0m0s', label: 'Every 12 hours' },
  { value: '24h0m0s', label: 'Once a day' },
]

const confidences: { value: Preferences['matchConfidence']; label: string; hint: string }[] = [
  { value: 'strict', label: 'Strict', hint: 'Only near-identical titles and authors. More books left for you to match.' },
  { value: 'medium', label: 'Medium', hint: 'Allows small differences like subtitles or punctuation.' },
  { value: 'loose', label: 'Loose', hint: 'Matches more on its own, with a higher chance of a wrong book.' },
]

export default function PreferencesCard() {
  const [prefs, setPrefs] = useState<Preferences | null>(null)
  const [busy, setBusy] = useState(false)
  const [result, setResult] = useState<{ kind: 'ok' | 'error'; text: string } | null>(null)

  useEffect(() => {
    fetchPreferences()
      .then(setPrefs)
      .catch((err) => setResult({ kind: 'error', text: errorMessage(err) }))
  }, [])

  if (prefs === null) {
    return (
      <Card title="Preferences" icon={<SlidersIcon />}>
        {result ? <Notice kind="error">{result.text}</Notice> : <p className="text-sm text-muted">Loading…</p>}
      </Card>
    )
  }

  const update = (patch: Partial<Preferences>) => {
    setPrefs({ ...prefs, ...patch })
    setResult(null)
  }

  const save = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setResult(null)
    try {
      setPrefs(await savePreferences(prefs))
      setResult({ kind: 'ok', text: 'Saved.' })
    } catch (err) {
      setResult({ kind: 'error', text: errorMessage(err) })
    } finally {
      setBusy(false)
    }
  }

  const origin = window.location.origin
  return (
    <Card title="Preferences" icon={<SlidersIcon />}>
      <form onSubmit={save} className="flex flex-col gap-5">
        <label className="flex flex-col gap-1.5">
          <span className="text-sm font-semibold">How often to sync</span>
          <select
            className={inputClass}
            value={prefs.syncInterval}
            onChange={(e) => update({ syncInterval: e.target.value })}
          >
            {intervals.map((i) => (
              <option key={i.value} value={i.value}>
                {i.label}
              </option>
            ))}
          </select>
        </label>

        <fieldset className="flex flex-col gap-1.5">
          <legend className="text-sm font-semibold mb-1.5">How sure a match must be</legend>
          <div className="grid grid-cols-3 gap-1 p-1 rounded-lg bg-paper border border-line">
            {confidences.map((c) => (
              <label
                key={c.value}
                className={`text-center text-sm rounded-md py-2 cursor-pointer transition-colors ${
                  prefs.matchConfidence === c.value ? 'bg-pine text-paper font-semibold' : 'text-muted hover:text-ink'
                }`}
              >
                <input
                  type="radio"
                  name="matchConfidence"
                  value={c.value}
                  checked={prefs.matchConfidence === c.value}
                  onChange={() => update({ matchConfidence: c.value })}
                  className="sr-only"
                />
                {c.label}
              </label>
            ))}
          </div>
          <p className="text-xs text-muted">{confidences.find((c) => c.value === prefs.matchConfidence)?.hint}</p>
        </fieldset>

        <label className="flex flex-col gap-1.5">
          <span className="text-sm font-semibold">Web UI address for notification links</span>
          <input
            className={inputClass}
            value={prefs.publicUrl}
            onChange={(e) => update({ publicUrl: e.target.value })}
            placeholder={origin}
            inputMode="url"
            autoComplete="off"
          />
          <span className="text-xs text-muted">
            Optional. Messages link here so you can match books in one tap.{' '}
            {prefs.publicUrl === '' && (
              <button type="button" className="text-pine-text underline decoration-dotted" onClick={() => update({ publicUrl: origin })}>
                Use {origin}
              </button>
            )}
          </span>
        </label>

        <div>
          <Button type="submit" busy={busy}>
            Save preferences
          </Button>
        </div>
        {result && <Notice kind={result.kind}>{result.text}</Notice>}
      </form>
    </Card>
  )
}
