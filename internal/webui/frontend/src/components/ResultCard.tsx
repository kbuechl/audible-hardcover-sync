import type { SearchResultItem, UnmatchedBook } from '../types'
import CoverThumb from './CoverThumb'

interface ResultCardProps {
  result: SearchResultItem
  book: UnmatchedBook
  onConfirm: (hardcoverBookId: number) => void
}

function isExactMatch(result: SearchResultItem, book: UnmatchedBook): boolean {
  return result.title.trim().toLowerCase() === book.title.trim().toLowerCase()
}

export default function ResultCard({ result, book, onConfirm }: ResultCardProps) {
  const match = isExactMatch(result, book)
  return (
    <div className="flex items-center justify-between gap-3 border border-line rounded-lg p-3 bg-paper">
      <div className="flex items-center gap-3 min-w-0">
        <CoverThumb title={result.title} coverUrl={result.coverUrl} size="sm" />
        <div className="min-w-0">
          <p className="font-serif font-semibold text-sm truncate">{result.title}</p>
          <p className="text-xs text-muted">
            {result.authorNames.join(', ')}
            {result.releaseYear ? ` · ${result.releaseYear}` : ''}
          </p>
        </div>
      </div>
      <div className="flex items-center gap-2 shrink-0">
        <a
          href={`https://hardcover.app/books/${result.slug}`}
          target="_blank"
          rel="noreferrer"
          className="text-xs text-pine-text underline decoration-dotted whitespace-nowrap"
        >
          View on Hardcover ↗
        </a>
        <button
          type="button"
          onClick={() => onConfirm(result.id)}
          className={`text-xs font-semibold rounded px-3 py-1.5 text-white whitespace-nowrap ${
            match ? 'bg-gold' : 'bg-pine'
          }`}
        >
          Confirm match
        </button>
      </div>
    </div>
  )
}
