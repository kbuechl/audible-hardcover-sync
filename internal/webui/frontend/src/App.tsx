import { useEffect, useState } from 'react'
import type { UnmatchedBook } from './types'
import { fetchUnmatched } from './api'
import Shelf from './components/Shelf'

export default function App() {
  const [books, setBooks] = useState<UnmatchedBook[]>([])
  const [selectedAsin, setSelectedAsin] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    fetchUnmatched()
      .then(setBooks)
      .finally(() => setLoading(false))
  }, [])

  return (
    <div className="max-w-5xl mx-auto px-6 py-8">
      <header className="flex items-baseline justify-between mb-7 flex-wrap gap-2">
        <div className="flex items-baseline gap-2">
          <h1 className="font-serif font-semibold text-2xl">Unmatched Shelf</h1>
          <span className="font-mono text-xs text-muted uppercase tracking-wide border border-line rounded-full px-2 py-0.5">
            manual match
          </span>
        </div>
        <p className="font-mono text-sm text-muted">
          <span className="text-burgundy font-bold">{books.length}</span> books waiting to be matched
        </p>
      </header>

      <div className="grid grid-cols-1 md:grid-cols-[320px_1fr] gap-6 items-start">
        <Shelf books={books} selectedAsin={selectedAsin} onSelect={setSelectedAsin} />
        {loading && <p className="text-muted text-sm">Loading…</p>}
        {!loading && selectedAsin === null && (
          <p className="text-muted text-sm p-4">Select a book to resolve it.</p>
        )}
      </div>
    </div>
  )
}
