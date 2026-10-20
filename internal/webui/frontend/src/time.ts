const MINUTE = 60_000
const HOUR = 60 * MINUTE
const DAY = 24 * HOUR

// timeAgo formats a past timestamp as a short phrase like "5 min ago".
export function timeAgo(iso: string, now: number = Date.now()): string {
  const diff = Math.max(0, now - new Date(iso).getTime())
  if (diff < MINUTE) return 'just now'
  if (diff < HOUR) return `${Math.floor(diff / MINUTE)} min ago`
  if (diff < DAY) {
    const h = Math.floor(diff / HOUR)
    return `${h} hour${h === 1 ? '' : 's'} ago`
  }
  const d = Math.floor(diff / DAY)
  return `${d} day${d === 1 ? '' : 's'} ago`
}
