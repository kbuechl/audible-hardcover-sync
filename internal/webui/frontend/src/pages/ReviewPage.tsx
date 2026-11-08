import { useCallback, useEffect, useState } from 'react'
import type { RemovePreview, ReviewDecision, ReviewFilter, ReviewItem, Status } from '../types'
import { fetchReview, previewRemove, removeFromHardcover, setReviewDecision } from '../api'
import { Button, Card, Notice } from '../components/ui'
import { inputClass, errorMessage } from '../components/uiStyles'
import { CheckIcon } from '../components/icons'
import ReviewRow from '../components/ReviewRow'
import RemoveDialog from '../components/RemoveDialog'
import Toast from '../components/Toast'

interface ReviewPageProps {
  status?: Status | null
  onChanged?: () => void
}

const filters: { id: ReviewFilter; label: string }[] = [
  { id: 'waiting', label: 'Waiting' },
  { id: 'synced', label: 'Synced' },
  { id: 'not_wanted', label: 'Not wanted' },
]

const SEARCH_DEBOUNCE_MS = 250

const emptyText: Record<ReviewFilter, string> = {
  waiting: 'No books are waiting for your review.',
  synced: 'No synced books to show.',
  not_wanted: 'No books are marked Not wanted.',
}

export default function ReviewPage({ onChanged = () => {} }: ReviewPageProps) {
  const [filter, setFilter] = useState<ReviewFilter>('waiting')
  const [query, setQuery] = useState('')
  const [q, setQ] = useState('')
  const [items, setItems] = useState<ReviewItem[]>([])
  const [loading, setLoading] = useState(true)
  const [selected, setSelected] = useState<ReadonlySet<string>>(new Set())
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [toast, setToast] = useState<string | null>(null)
  const [removing, setRemoving] = useState<{ item: ReviewItem; preview: RemovePreview } | null>(null)
  const hideToast = useCallback(() => setToast(null), [])

  useEffect(() => {
    const timer = window.setTimeout(() => setQ(query.trim()), SEARCH_DEBOUNCE_MS)
    return () => window.clearTimeout(timer)
  }, [query])

  const load = useCallback(async () => {
    try {
      const list = await fetchReview(filter, q)
      setItems(list)
      // Keep only selections that are still listed.
      setSelected((prev) => new Set(list.filter((i) => prev.has(i.asin)).map((i) => i.asin)))
      setError(null)
    } catch (err) {
      setError(`Couldn't load the list — ${errorMessage(err)}.`)
    } finally {
      setLoading(false)
    }
  }, [filter, q])

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    void load()
  }, [load])

  const changeFilter = (next: ReviewFilter) => {
    if (next === filter) return
    setFilter(next)
    setItems([])
    setSelected(new Set())
    setLoading(true)
  }

  const toggle = (asin: string) =>
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(asin)) next.delete(asin)
      else next.add(asin)
      return next
    })

  const allSelected = items.length > 0 && selected.size === items.length
  const toggleAll = () => setSelected(allSelected ? new Set() : new Set(items.map((i) => i.asin)))

  const decide = async (asins: string[], decision: ReviewDecision) => {
    setBusy(true)
    setError(null)
    try {
      await setReviewDecision(asins, decision)
      const n = asins.length
      const books = n === 1 ? '1 book' : `${n} books`
      setToast(decision === 'sync' ? `${books} will sync on the next run.` : `${books} marked Not wanted.`)
      await load()
      onChanged()
    } catch (err) {
      setError(`Couldn't save that — ${errorMessage(err)}. Try again.`)
    } finally {
      setBusy(false)
    }
  }

  const openRemove = async (item: ReviewItem) => {
    setError(null)
    try {
      setRemoving({ item, preview: await previewRemove(item.asin) })
    } catch (err) {
      setError(`Couldn't check what Remove would delete — ${errorMessage(err)}.`)
    }
  }

  const confirmRemove = async () => {
    if (removing === null) return
    await removeFromHardcover(removing.item.asin)
    setToast(`Removed “${removing.item.title}” from Hardcover.`)
    setRemoving(null)
    await load()
    onChanged()
  }

  const pickedAsins = [...selected]
  const canSync = filter !== 'synced'
  const canReject = filter !== 'not_wanted'

  return (
    <div className="flex flex-col gap-4">
      <h1 className="font-serif font-semibold text-2xl">Review</h1>

      <div role="tablist" aria-label="Filter" className="flex gap-1 border-b border-line">
        {filters.map((f) => (
          <button
            key={f.id}
            type="button"
            role="tab"
            aria-selected={filter === f.id}
            onClick={() => changeFilter(f.id)}
            className={`text-sm px-3 py-2 -mb-px border-b-2 ${
              filter === f.id ? 'border-pine text-ink font-semibold' : 'border-transparent text-muted hover:text-ink'
            }`}
          >
            {f.label}
          </button>
        ))}
      </div>

      <input
        type="search"
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        placeholder="Search titles"
        aria-label="Search titles"
        className={inputClass}
      />

      {error !== null && <Notice kind="error">{error}</Notice>}

      {loading ? (
        <p className="text-muted text-sm">Loading…</p>
      ) : items.length === 0 ? (
        <div className="bg-panel border border-line rounded-xl px-6 py-10 text-center shadow-sm flex flex-col items-center gap-3">
          {filter === 'waiting' && q === '' && (
            <span className="grid place-items-center size-14 rounded-full bg-pine-soft text-pine-text">
              <CheckIcon />
            </span>
          )}
          <h2 className="font-serif text-xl font-semibold">
            {filter === 'waiting' && q === '' ? 'All caught up' : 'Nothing here'}
          </h2>
          <p className="text-sm text-muted">{q === '' ? emptyText[filter] : 'No books match that search.'}</p>
        </div>
      ) : (
        <Card title={filters.find((f) => f.id === filter)?.label ?? ''}>
          <div className="flex flex-wrap items-center gap-2 pb-3 border-b border-line">
            <label className="flex items-center gap-2 text-sm mr-auto">
              <input
                type="checkbox"
                checked={allSelected}
                onChange={toggleAll}
                className="size-4 accent-pine"
              />
              Select all ({items.length})
            </label>
            {canSync && (
              <Button
                disabled={pickedAsins.length === 0}
                busy={busy}
                onClick={() => decide(pickedAsins, 'sync')}
              >
                Sync selected
              </Button>
            )}
            {canReject && (
              <Button
                kind="secondary"
                disabled={pickedAsins.length === 0}
                busy={busy}
                onClick={() => decide(pickedAsins, 'not_wanted')}
              >
                Not wanted selected
              </Button>
            )}
          </div>
          <ul>
            {items.map((item) => (
              <ReviewRow key={item.asin} item={item} selected={selected.has(item.asin)} onToggle={() => toggle(item.asin)}>
                {canSync && (
                  <Button disabled={busy} onClick={() => decide([item.asin], 'sync')}>
                    Sync
                  </Button>
                )}
                {filter === 'synced' && item.canRemove && (
                  <Button kind="danger" disabled={busy} onClick={() => openRemove(item)}>
                    Remove from Hardcover
                  </Button>
                )}
                {canReject && (
                  <Button kind="secondary" disabled={busy} onClick={() => decide([item.asin], 'not_wanted')}>
                    Not wanted
                  </Button>
                )}
              </ReviewRow>
            ))}
          </ul>
        </Card>
      )}

      {removing !== null && (
        <RemoveDialog
          title={removing.item.title}
          preview={removing.preview}
          onConfirm={confirmRemove}
          onCancel={() => setRemoving(null)}
        />
      )}
      {toast !== null && <Toast message={toast} onDone={hideToast} />}
    </div>
  )
}
