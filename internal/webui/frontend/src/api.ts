import type { UnmatchedBook, SearchResultItem, Status, NotifyTarget, Preferences } from './types'

async function fetchOK(input: RequestInfo, init?: RequestInit): Promise<Response> {
  const res = await fetch(input, init)
  if (!res.ok) {
    const body = await res.text()
    // Settings endpoints reply {"error": "..."}; show just that message.
    try {
      const parsed = JSON.parse(body) as { error?: string }
      if (parsed.error) throw new Error(parsed.error)
    } catch (err) {
      if (err instanceof Error && !(err instanceof SyntaxError)) throw err
    }
    throw new Error(`${res.status} ${res.statusText}: ${body}`)
  }
  return res
}

async function fetchJSON<T>(input: RequestInfo, init?: RequestInit): Promise<T> {
  const res = await fetchOK(input, init)
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
  // /api/resolve replies 200 with an empty body, so don't parse JSON.
  await fetchOK('/api/resolve', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ asin, hardcoverBookId }),
  })
}

function send(method: string, url: string, body?: unknown): Promise<Response> {
  return fetchOK(url, {
    method,
    headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
}

export function fetchStatus(): Promise<Status> {
  return fetchJSON('/api/status')
}

export async function startAudibleLogin(): Promise<string> {
  const res = await send('POST', '/api/audible/login')
  const { url } = (await res.json()) as { url: string }
  return url
}

export async function finishAudibleLogin(redirectUrl: string): Promise<void> {
  await send('POST', '/api/audible/login/finish', { redirectUrl })
}

export async function logoutAudible(): Promise<void> {
  await send('POST', '/api/audible/logout')
}

export async function setHardcoverToken(token: string): Promise<string> {
  const res = await send('PUT', '/api/hardcover/token', { token })
  const { username } = (await res.json()) as { username: string }
  return username
}

export async function syncNow(): Promise<void> {
  await send('POST', '/api/sync')
}

export async function resetSync(): Promise<void> {
  await send('POST', '/api/sync/reset')
}

export function fetchNotifyTargets(): Promise<NotifyTarget[]> {
  return fetchJSON('/api/notifications')
}

export async function addNotifyURL(url: string): Promise<void> {
  await send('POST', '/api/notifications', { url })
}

export async function removeNotifyTarget(id: number): Promise<void> {
  await send('DELETE', `/api/notifications/${id}`)
}

export async function testNotifyTarget(id: number): Promise<void> {
  await send('POST', `/api/notifications/${id}/test`)
}

export async function testNotifyURL(url: string): Promise<void> {
  await send('POST', '/api/notifications/test', { url })
}

export function fetchPreferences(): Promise<Preferences> {
  return fetchJSON('/api/preferences')
}

export async function savePreferences(p: Preferences): Promise<Preferences> {
  const res = await send('PUT', '/api/preferences', p)
  return (await res.json()) as Preferences
}
