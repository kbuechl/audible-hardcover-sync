import type { UnmatchedBook, SearchResultItem } from './types'

async function fetchJSON<T>(input: RequestInfo, init?: RequestInit): Promise<T> {
  const res = await fetch(input, init)
  if (!res.ok) {
    const body = await res.text()
    throw new Error(`${res.status} ${res.statusText}: ${body}`)
  }
  return res.json() as Promise<T>
}

export function fetchUnmatched(): Promise<UnmatchedBook[]> {
  return fetchJSON('/api/unmatched')
}

export function fetchUnmatchedDetail(asin: string): Promise<UnmatchedBook> {
  return fetchJSON(`/api/unmatched/${encodeURIComponent(asin)}`)
}

export function searchHardcover(asin: string, query: string): Promise<SearchResultItem[]> {
  const params = new URLSearchParams({ asin, q: query })
  return fetchJSON(`/api/search?${params.toString()}`)
}

export async function resolveMatch(asin: string, hardcoverBookId: number): Promise<void> {
  await fetchJSON('/api/resolve', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ asin, hardcoverBookId }),
  })
}
