import { useEffect } from 'react'

const TOAST_MS = 4000

interface ToastProps {
  message: string
  onDone: () => void
}

// Toast shows a short confirmation in the corner and hides itself.
export default function Toast({ message, onDone }: ToastProps) {
  useEffect(() => {
    const timer = window.setTimeout(onDone, TOAST_MS)
    return () => window.clearTimeout(timer)
  }, [message, onDone])

  return (
    <div
      role="status"
      className="fixed bottom-4 inset-x-4 sm:inset-x-auto sm:bottom-6 sm:right-6 z-50 sm:max-w-sm rounded-lg border border-pine bg-pine-soft text-pine-text px-4 py-3 text-sm shadow-lg"
    >
      {message}
    </div>
  )
}
