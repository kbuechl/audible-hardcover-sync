import { describe, it, expect, beforeEach } from 'vitest'
import { applyTheme, currentTheme } from './theme'

describe('theme', () => {
  beforeEach(() => {
    localStorage.clear()
    delete document.documentElement.dataset.theme
  })

  it('defaults to light when the system has no preference', () => {
    expect(currentTheme()).toBe('light')
  })

  it('saves and applies a picked theme', () => {
    applyTheme('dark')
    expect(document.documentElement.dataset.theme).toBe('dark')
    expect(currentTheme()).toBe('dark')
  })
})
