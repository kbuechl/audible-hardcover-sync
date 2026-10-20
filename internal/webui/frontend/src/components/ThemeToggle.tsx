import { useState } from 'react'
import { applyTheme, currentTheme, type Theme } from '../theme'
import { MoonIcon, SunIcon } from './icons'

// ThemeToggle is a sun/moon switch that flips light and dark for this browser.
export default function ThemeToggle() {
  const [theme, setTheme] = useState<Theme>(currentTheme)
  const dark = theme === 'dark'
  const flip = () => {
    const next: Theme = dark ? 'light' : 'dark'
    applyTheme(next)
    setTheme(next)
  }
  return (
    <button
      type="button"
      role="switch"
      aria-checked={dark}
      aria-label="Dark mode"
      title={dark ? 'Dark mode on' : 'Dark mode off'}
      onClick={flip}
      className="relative flex items-center w-14 h-7 rounded-full border border-line bg-panel shrink-0"
    >
      <span
        aria-hidden="true"
        className={`absolute top-0.5 left-0.5 size-[22px] rounded-full bg-pine shadow-sm transition-transform duration-200 ${
          dark ? 'translate-x-7' : ''
        }`}
      />
      <span
        className={`relative z-10 grid place-items-center w-1/2 h-full ${dark ? 'text-muted' : 'text-paper'}`}
      >
        <SunIcon />
      </span>
      <span
        className={`relative z-10 grid place-items-center w-1/2 h-full ${dark ? 'text-paper' : 'text-muted'}`}
      >
        <MoonIcon />
      </span>
    </button>
  )
}
