export type Theme = 'light' | 'dark'

const KEY = 'theme'

// currentTheme is the saved choice, or the system setting if none.
export function currentTheme(): Theme {
  try {
    const t = localStorage.getItem(KEY)
    if (t === 'light' || t === 'dark') return t
  } catch {
    // Storage blocked: fall back to the system setting.
  }
  return window.matchMedia?.('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

// applyTheme sets the page theme and remembers it in this browser.
export function applyTheme(theme: Theme) {
  document.documentElement.dataset.theme = theme
  try {
    localStorage.setItem(KEY, theme)
  } catch {
    // Storage blocked: the choice lasts until the page reloads.
  }
}
