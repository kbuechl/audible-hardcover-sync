import { useMemo } from 'react'
import type { UnmatchedBook } from '../types'
import { useDebouncedSearch } from '../hooks/useDebouncedSearch'
import CoverThumb from './CoverThumb'
import ResultCard, { ResultSkeleton } from './ResultCard'
import { rankResults } from '../match'

interface ResolvePanelProps {
  book: UnmatchedBook
  onConfirm: (hardcoverBookId: number) => void
  confirming?: boolean
}

export default function ResolvePanel({ book, onConfirm, confirming = false }: ResolvePanelProps) {
  const { query, setQuery, results, loading } = useDebouncedSearch(book.asin, book.title)
  const { ranked, bestFound } = useMemo(() => rankResults(results, book.title, book.author), [results, book.title, book.author])

  return (
    <div className="bg-panel border border-line rounded-xl p-4 sm:p-7">
      <p className="font-mono text-[0.68rem] uppercase tracking-wide text-muted mb-2">Resolve match</p>
      <div className="flex gap-4 items-start mb-6">
        <CoverThumb title={book.title || book.asin} coverUrl={book.coverUrl} size="lg" />
        <div className="min-w-0">
          <h2 className="font-serif text-xl sm:text-2xl font-semibold break-words">{book.title || book.asin}</h2>
          <p className="text-muted">
            {book.author} <span className="font-mono text-xs ml-2">{book.asin}</span>
          </p>
        </div>
      </div>

      <input
        className="w-full text-base sm:text-sm px-3.5 py-2.5 border-[1.5px] border-pine rounded-lg bg-paper outline-none focus:ring-2 focus:ring-pine/30"
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        placeholder="Search Hardcover by title or author"
      />
      <p className="text-xs text-muted mt-2 mb-4">
        Prefilled from Audible. Results update as you type.
      </p>

      {loading && (
        <div className="flex flex-col gap-2" role="status" aria-label="Searching">
          <ResultSkeleton />
          <ResultSkeleton />
          <ResultSkeleton />
        </div>
      )}
      {!loading && results.length === 0 && (
        <p className="text-sm text-muted">No matches yet — try adjusting the search.</p>
      )}
      <div className="flex flex-col gap-2">
        {!loading && ranked.map((r, i) => (
          <ResultCard key={r.id} result={r} best={bestFound && i === 0} onConfirm={onConfirm} confirming={confirming} />
        ))}
      </div>
    </div>
  )
}
