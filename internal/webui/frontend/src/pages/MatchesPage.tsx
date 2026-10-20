import { useCallback, useEffect, useRef, useState } from 'react'
import type { Status, UnmatchedBook } from '../types'
import { fetchUnmatched, fetchUnmatchedDetail, resolveMatch } from '../api'
import Shelf from '../components/Shelf'
import ResolvePanel from '../components/ResolvePanel'
import Toast from '../components/Toast'
import AllCaughtUp from '../components/AllCaughtUp'

interface MatchesPageProps {
  status?: Status | null
  onSyncStarted?: () => void
}

export default function MatchesPage({ status = null, onSyncStarted = () => {} }: MatchesPageProps) {
  const [books, setBooks] = useState<UnmatchedBook[]>([])
  const [selectedAsin, setSelectedAsin] = useState<string | null>(null)
  const [fetchedBook, setFetchedBook] = useState<UnmatchedBook | null>(null)
  const [loading, setLoading] = useState(true)
  const [confirming, setConfirming] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [toast, setToast] = useState<string | null>(null)
  const hideToast = useCallback(() => setToast(null), [])
  const panelRef = useRef<HTMLDivElement>(null)
  // Only show details that belong to the current selection.
  const selectedBook = fetchedBook !== null && fetchedBook.asin === selectedAsin ? fetchedBook : null

  // On phones the panel sits below the shelf; bring it into view.
  useEffect(() => {
    if (selectedBook === null || window.matchMedia?.('(min-width: 768px)').matches) return
    panelRef.current?.scrollIntoView?.({ behavior: 'smooth', block: 'start' })
  }, [selectedBook])

  const reloadShelf = () => fetchUnmatched().then(setBooks)

  useEffect(() => {
    reloadShelf().finally(() => setLoading(false))
  }, [])

  useEffect(() => {
    if (selectedAsin !== null) fetchUnmatchedDetail(selectedAsin).then(setFetchedBook)
  }, [selectedAsin])

  const handleConfirm = async (hardcoverBookId: number) => {
    if (selectedAsin === null) return
    setConfirming(true)
    setError(null)
    try {
      await resolveMatch(selectedAsin, hardcoverBookId)
      const title = selectedBook?.title
      setToast(title ? `Matched “${title}”. It'll be added to Hardcover on the next sync.` : "Matched. It'll be added to Hardcover on the next sync.")
      setSelectedAsin(null)
      await reloadShelf()
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err)
      setError(`Couldn't confirm this match — ${message}. Try again.`)
    } finally {
      setConfirming(false)
    }
  }

  return (
    <div>
      <header className="flex items-baseline justify-between mb-7 flex-wrap gap-2">
        <div className="flex items-baseline gap-2">
          <h1 className="font-serif font-semibold text-2xl">Unmatched Shelf</h1>
          <span className="font-mono text-xs text-muted uppercase tracking-wide border border-line rounded-full px-2 py-0.5">
            manual match
          </span>
        </div>
        {books.length > 0 && (
          <p className="font-mono text-sm text-muted">
            <span className="text-burgundy font-bold">{books.length}</span>{' '}
            {books.length === 1 ? 'book' : 'books'} waiting to be matched
          </p>
        )}
      </header>

      {error !== null && (
        <p className="font-mono text-xs bg-burgundy-soft text-burgundy rounded-lg px-4 py-3 mb-6">
          {error}
        </p>
      )}

      {!loading && books.length === 0 ? (
        <AllCaughtUp status={status} onSyncStarted={onSyncStarted} />
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-[320px_1fr] gap-6 items-start">
          <Shelf books={books} selectedAsin={selectedAsin} onSelect={setSelectedAsin} />
          {loading && <p className="text-muted text-sm">Loading…</p>}
          {!loading && selectedBook === null && (
            <p className="hidden md:grid place-items-center min-h-48 border border-dashed border-line rounded-xl text-muted text-sm p-6">
              Pick a book on the left to find it on Hardcover.
            </p>
          )}
          {selectedBook !== null && (
            <div ref={panelRef} className="scroll-mt-4 min-w-0">
              <ResolvePanel book={selectedBook} onConfirm={handleConfirm} confirming={confirming} />
            </div>
          )}
        </div>
      )}

      {toast !== null && <Toast message={toast} onDone={hideToast} />}
    </div>
  )
}
