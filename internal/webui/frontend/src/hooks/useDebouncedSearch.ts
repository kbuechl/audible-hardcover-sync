import { useEffect, useRef, useState } from 'react'
import { searchHardcover } from '../api'
import type { SearchResultItem } from '../types'

const DEBOUNCE_MS = 300

/**
 * Fires an immediate search using `initialQuery` (the Audible-title prefill)
 * as soon as `asin` is set, then debounces every subsequent `setQuery` call
 * by DEBOUNCE_MS. Cancels a stale in-flight response if a newer search has
 * since started, so a slow early response can never overwrite fresher
 * results.
 *
 * Correctness note: this hook is reused across book selections on the same
 * mounted <ResolvePanel> (asin/initialQuery change as props, the component
 * doesn't remount). A naive "one effect resets query+isFirstRun, a second
 * effect fires when isFirstRun" split has a real bug here: React runs both
 * effects in the same commit before the query-reset's re-render happens, so
 * the "immediate search" effect can see the *previous* book's `query` value
 * still in its closure and fire a search carrying the wrong book's text.
 * This implementation avoids that by tracking `lastAsin` explicitly and
 * refusing to search on an asin transition until `query` has actually
 * caught up to `initialQuery` in a render — see the "re-fires immediately
 * with the NEW prefilled query" test, which is a regression test for
 * exactly this failure mode.
 */
export function useDebouncedSearch(asin: string | null, initialQuery: string) {
  const [query, setQuery] = useState(initialQuery)
  const [results, setResults] = useState<SearchResultItem[]>([])
  const [loading, setLoading] = useState(false)
  const requestID = useRef(0)
  const lastAsin = useRef<string | null>(null)

  // Keep `query` in sync with `initialQuery` whenever the selected book
  // changes. On the very first mount for a given asin this is a no-op
  // (query already equals initialQuery via useState's initializer), so it
  // never triggers an extra render in that case.
  useEffect(() => {
    if (asin !== lastAsin.current) {
      setQuery(initialQuery)
    }
  }, [asin, initialQuery])

  useEffect(() => {
    if (asin === null) return

    const isNewAsin = asin !== lastAsin.current
    // On an asin transition, wait for `query` to actually equal
    // `initialQuery` before searching — the sync effect above will trigger
    // exactly one more run of this effect once that catches up. Searching
    // here with a `query` that hasn't caught up yet would use the previous
    // book's stale text.
    if (isNewAsin && query !== initialQuery) {
      return
    }

    lastAsin.current = asin
    const thisRequest = ++requestID.current
    const runSearch = () => {
      setLoading(true)
      searchHardcover(asin, query)
        .then((r) => {
          if (requestID.current === thisRequest) {
            setResults(r)
          }
        })
        .finally(() => {
          if (requestID.current === thisRequest) {
            setLoading(false)
          }
        })
    }

    if (isNewAsin) {
      runSearch()
      return
    }

    const timer = setTimeout(runSearch, DEBOUNCE_MS)
    return () => clearTimeout(timer)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [asin, query, initialQuery])

  return { query, setQuery, results, loading }
}
