import type { ReactNode } from 'react'

export function Card({
  title,
  icon,
  badge,
  children,
}: {
  title: string
  icon?: ReactNode
  badge?: ReactNode
  children: ReactNode
}) {
  return (
    <section className="bg-panel border border-line rounded-xl p-4 sm:p-6 shadow-sm">
      <div className="flex items-center justify-between gap-3 mb-4">
        <h2 className="font-serif text-xl font-semibold flex items-center gap-2.5">
          {icon && (
            <span className="grid place-items-center size-8 rounded-lg bg-pine-soft text-pine-text shrink-0">{icon}</span>
          )}
          {title}
        </h2>
        {badge}
      </div>
      {children}
    </section>
  )
}

export function Badge({ ok, children }: { ok: boolean; children: ReactNode }) {
  return (
    <span
      className={`font-mono text-[0.68rem] uppercase tracking-wide rounded-full px-2.5 py-1 ${
        ok ? 'bg-pine-soft text-pine-text' : 'bg-burgundy-soft text-burgundy'
      }`}
    >
      {children}
    </span>
  )
}

type ButtonKind = 'primary' | 'secondary' | 'danger'

const buttonStyles: Record<ButtonKind, string> = {
  primary: 'bg-pine text-paper hover:brightness-110',
  secondary: 'border border-line bg-paper text-ink hover:border-pine',
  danger: 'border border-burgundy text-burgundy bg-paper hover:bg-burgundy-soft',
}

export function Spinner() {
  return (
    <span
      aria-hidden="true"
      className="inline-block size-3.5 rounded-full border-2 border-current border-r-transparent animate-spin"
    />
  )
}

export function Button({
  kind = 'primary',
  type = 'button',
  disabled,
  busy,
  onClick,
  children,
}: {
  kind?: ButtonKind
  type?: 'button' | 'submit'
  disabled?: boolean
  busy?: boolean
  onClick?: () => void
  children: ReactNode
}) {
  return (
    <button
      type={type}
      onClick={onClick}
      disabled={disabled || busy}
      aria-busy={busy || undefined}
      className={`inline-flex items-center justify-center gap-2 text-sm font-semibold rounded-lg px-4 py-2.5 sm:py-2 whitespace-nowrap transition disabled:opacity-50 disabled:cursor-not-allowed ${buttonStyles[kind]}`}
    >
      {busy && <Spinner />}
      {children}
    </button>
  )
}

export function Notice({ kind, children }: { kind: 'error' | 'ok' | 'info'; children: ReactNode }) {
  const style =
    kind === 'error' ? 'bg-burgundy-soft text-burgundy' : kind === 'ok' ? 'bg-pine-soft text-pine-text' : 'bg-paper text-muted'
  return (
    <p role={kind === 'error' ? 'alert' : 'status'} className={`text-sm rounded-lg px-4 py-3 ${style}`}>
      {children}
    </p>
  )
}
