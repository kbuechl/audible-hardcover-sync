import type { UnmatchedBook } from '../types'
import CoverThumb from './CoverThumb'

interface SpineCardProps {
  book: UnmatchedBook
  active: boolean
  onSelect: (asin: string) => void
}

export default function SpineCard({ book, active, onSelect }: SpineCardProps) {
  const pillClass =
    book.status === 'finished'
      ? 'bg-pine-soft text-pine-text'
      : 'bg-burgundy-soft text-burgundy'
  const pillLabel = book.status === 'finished' ? 'Finished' : 'In progress'

  return (
    <button
      type="button"
      onClick={() => onSelect(book.asin)}
      className={`flex items-stretch text-left bg-panel border rounded-lg overflow-hidden w-full transition-colors ${
        active ? 'border-pine ring-1 ring-pine' : 'border-line hover:border-pine'
      }`}
    >
      <div className={`w-1.5 shrink-0 ${active ? 'bg-gold' : 'bg-pine'}`} />
      <div className="p-3 flex gap-3 items-center min-w-0 flex-1">
        <CoverThumb title={book.title || book.asin} coverUrl={book.coverUrl} size="sm" />
        <div className="min-w-0 flex-1">
          <p className="font-serif font-semibold text-sm truncate">{book.title || book.asin}</p>
          <p className="text-xs text-muted mb-1">{book.author}</p>
          <div className="flex items-center gap-2">
            <span className={`font-mono text-[0.65rem] uppercase tracking-wide px-2 py-0.5 rounded-full font-semibold ${pillClass}`}>
              {pillLabel}
            </span>
            <span className="font-mono text-[0.68rem] text-muted">{book.asin}</span>
          </div>
        </div>
      </div>
    </button>
  )
}
