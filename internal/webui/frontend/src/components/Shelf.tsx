import type { UnmatchedBook } from '../types'
import SpineCard from './SpineCard'

interface ShelfProps {
  books: UnmatchedBook[]
  selectedAsin: string | null
  onSelect: (asin: string) => void
}

export default function Shelf({ books, selectedAsin, onSelect }: ShelfProps) {
  if (books.length === 0) {
    return <p className="text-muted text-sm p-4">Nothing to resolve right now.</p>
  }
  return (
    <div className="flex flex-col gap-2">
      {books.map((book) => (
        <SpineCard key={book.asin} book={book} active={book.asin === selectedAsin} onSelect={onSelect} />
      ))}
    </div>
  )
}
