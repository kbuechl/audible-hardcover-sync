import type { SearchResultItem } from '../types'
import CoverThumb from './CoverThumb'

interface ResultCardProps {
  result: SearchResultItem
  best?: boolean
  onConfirm: (hardcoverBookId: number) => void
  confirming?: boolean
}

export default function ResultCard({ result, best = false, onConfirm, confirming = false }: ResultCardProps) {
  return (
    <div
      className={`flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 border rounded-lg p-3 bg-paper ${
        best ? 'border-pine ring-1 ring-pine/40' : 'border-line'
      }`}
    >
      <div className="flex items-center gap-3 min-w-0 flex-1">
        <CoverThumb title={result.title} coverUrl={result.coverUrl} size="sm" />
        <div className="min-w-0 flex-1">
          {best && (
            <span className="inline-block font-mono text-[0.62rem] uppercase tracking-wide font-semibold rounded-full px-2 py-0.5 mb-1 bg-gold/20 text-gold">
              Best match
            </span>
          )}
          <p className="font-serif font-semibold text-sm break-words sm:truncate">{result.title}</p>
          <p className="text-xs text-muted">
            {result.authorNames.join(', ')}
            {result.releaseYear ? ` · ${result.releaseYear}` : ''}
          </p>
        </div>
      </div>
      <div className="flex items-center justify-between sm:justify-end gap-3 shrink-0">
        <a
          href={`https://hardcover.app/books/${result.slug}`}
          target="_blank"
          rel="noreferrer"
          className="text-xs text-pine-text underline decoration-dotted whitespace-nowrap py-2"
        >
          View on Hardcover ↗
        </a>
        <button
          type="button"
          onClick={() => onConfirm(result.id)}
          disabled={confirming}
          className={`text-xs font-semibold rounded-lg px-4 py-2.5 sm:px-3 sm:py-1.5 whitespace-nowrap transition disabled:opacity-50 disabled:cursor-not-allowed ${
            best ? 'bg-pine text-paper hover:brightness-110' : 'border border-pine text-pine-text hover:bg-pine-soft'
          }`}
        >
          Confirm match
        </button>
      </div>
    </div>
  )
}

export function ResultSkeleton() {
  return (
    <div className="flex items-center gap-3 border border-line rounded-lg p-3 bg-paper animate-pulse" aria-hidden="true">
      <div className="w-10 h-[58px] rounded bg-line shrink-0" />
      <div className="flex-1 flex flex-col gap-2">
        <div className="h-3 w-2/3 rounded bg-line" />
        <div className="h-2.5 w-1/3 rounded bg-line" />
      </div>
    </div>
  )
}
