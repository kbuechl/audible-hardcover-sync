import type { Status } from '../types'
import { describeStatus, type Tone } from '../status'

const tones: Record<Tone, { pill: string; dot: string }> = {
  ok: { pill: 'bg-pine-soft text-pine-text', dot: 'bg-pine' },
  warn: { pill: 'bg-burgundy-soft text-burgundy', dot: 'bg-burgundy' },
  busy: { pill: 'bg-pine-soft text-pine-text', dot: 'bg-pine animate-pulse' },
  idle: { pill: 'bg-panel text-muted border border-line', dot: 'bg-muted' },
}

// StatusPill shows the app's health in the header, on every tab.
export default function StatusPill({ status }: { status: Status | null }) {
  if (status === null) return null
  const { tone, text, short, href } = describeStatus(status)
  const style = tones[tone]
  return (
    <a
      href={href}
      title={text}
      className={`inline-flex items-center gap-2 rounded-full px-3 py-1 text-xs font-semibold whitespace-nowrap ${style.pill}`}
    >
      <span className={`size-2 rounded-full ${style.dot}`} aria-hidden="true" />
      <span className="sm:hidden">{short}</span>
      <span className="hidden sm:inline">{text}</span>
    </a>
  )
}
