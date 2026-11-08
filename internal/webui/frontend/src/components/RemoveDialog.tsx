import { useEffect, useState } from 'react'
import type { RemovePreview } from '../types'
import { Button, Notice } from './ui'
import { errorMessage } from './uiStyles'

interface RemoveDialogProps {
  title: string
  preview: RemovePreview
  onConfirm: () => Promise<void>
  onCancel: () => void
}

// RemoveDialog names what Remove from Hardcover will delete and waits for
// an explicit confirm.
export default function RemoveDialog({ title, preview, onConfirm, onCancel }: RemoveDialogProps) {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && !busy) onCancel()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [busy, onCancel])

  const confirm = async () => {
    setBusy(true)
    setError(null)
    try {
      await onConfirm()
    } catch (err) {
      setError(errorMessage(err))
      setBusy(false)
    }
  }

  return (
    <div className="fixed inset-0 z-40 grid place-items-center bg-black/40 p-4">
      <div
        role="dialog"
        aria-modal="true"
        aria-label={`Remove ${title} from Hardcover`}
        className="bg-panel border border-line rounded-xl p-5 sm:p-6 shadow-lg w-full max-w-md max-h-full overflow-y-auto flex flex-col gap-3"
      >
        <h2 className="font-serif text-xl font-semibold">Remove “{title}” from Hardcover?</h2>
        {preview.deletes.length > 0 && (
          <>
            <p className="text-sm">This deletes what the sync added:</p>
            <ul className="list-disc pl-5 text-sm">
              {preview.deletes.map((d) => (
                <li key={d}>{d}</li>
              ))}
            </ul>
          </>
        )}
        {preview.alsoDeletes.length > 0 && (
          <>
            <p className="text-sm text-burgundy font-semibold">This also deletes things you added on Hardcover:</p>
            <ul className="list-disc pl-5 text-sm text-burgundy">
              {preview.alsoDeletes.map((d) => (
                <li key={d}>{d}</li>
              ))}
            </ul>
          </>
        )}
        <p className="text-sm text-muted">The book will be marked Not wanted so it isn't added back.</p>
        {error !== null && <Notice kind="error">{error}</Notice>}
        <div className="flex flex-col-reverse sm:flex-row sm:justify-end gap-2 mt-2">
          <Button kind="secondary" onClick={onCancel} disabled={busy}>
            Keep it
          </Button>
          <Button kind="danger" onClick={confirm} busy={busy}>
            Remove from Hardcover
          </Button>
        </div>
      </div>
    </div>
  )
}
