import type { UnmatchedBook } from '../types'
import { useDebouncedSearch } from '../hooks/useDebouncedSearch'
import CoverThumb from './CoverThumb'
import ResultCard from './ResultCard'

interface ResolvePanelProps {
  book: UnmatchedBook
  onConfirm: (hardcoverBookId: number) => void
  confirming?: boolean
}

export default function ResolvePanel({ book, onConfirm, confirming = false }: ResolvePanelProps) {
  const { query, setQuery, results, loading } = useDebouncedSearch(book.asin, book.title)

  return (
    <div className="bg-panel border border-line rounded-xl p-7">
      <p className="font-mono text-[0.68rem] uppercase tracking-wide text-muted mb-2">Resolve match</p>
      <div className="flex gap-4 items-start mb-6">
        <CoverThumb title={book.title || book.asin} size="lg" />
        <div>
          <h2 className="font-serif text-2xl font-semibold">{book.title || book.asin}</h2>
          <p className="text-muted">
            {book.author} <span className="font-mono text-xs ml-2">{book.asin}</span>
          </p>
        </div>
      </div>

      <input
        className="w-full text-sm px-3.5 py-2.5 border-[1.5px] border-pine rounded-lg bg-paper outline-none"
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        placeholder="Search Hardcover by title or author"
      />
      <p className="text-xs text-muted mt-2 mb-4">
        Prefilled from Audible. Results update as you type.
      </p>

      {loading && <p className="text-sm text-muted">Searching…</p>}
      {!loading && results.length === 0 && (
        <p className="text-sm text-muted">No matches yet — try adjusting the search.</p>
      )}
      <div className="flex flex-col gap-2">
        {results.map((r) => (
          <ResultCard key={r.id} result={r} book={book} onConfirm={onConfirm} confirming={confirming} />
        ))}
      </div>
    </div>
  )
}
