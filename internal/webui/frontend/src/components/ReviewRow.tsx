import type { ReactNode } from 'react'
import type { ReviewItem } from '../types'
import CoverThumb from './CoverThumb'

interface ReviewRowProps {
  item: ReviewItem
  selected: boolean
  onToggle: () => void
  children: ReactNode
}

// ReviewRow is one book: checkbox, cover, details and its action buttons.
export default function ReviewRow({ item, selected, onToggle, children }: ReviewRowProps) {
  return (
    <li className="flex items-start gap-3 py-3 border-b border-line last:border-b-0">
      <input
        type="checkbox"
        checked={selected}
        onChange={onToggle}
        aria-label={`Select ${item.title}`}
        className="mt-1 size-4 shrink-0 accent-pine"
      />
      <CoverThumb title={item.title} coverUrl={item.coverUrl} />
      <div className="min-w-0 flex-1">
        <p className="font-serif font-semibold leading-snug">{item.title}</p>
        <p className="text-sm text-muted truncate">{item.author}</p>
        <p className="font-mono text-xs text-muted mt-1 flex items-center gap-2 flex-wrap">
          <span>
            {item.status}
            {item.percentComplete > 0 && ` · ${Math.round(item.percentComplete)}%`}
          </span>
          {item.shared && (
            <span className="uppercase tracking-wide border border-line rounded-full px-2 py-0.5">Shared</span>
          )}
        </p>
        <div className="flex flex-wrap gap-2 mt-2">{children}</div>
      </div>
    </li>
  )
}
