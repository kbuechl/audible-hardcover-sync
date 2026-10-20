import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import App from './App'
import * as api from './api'

const connectedStatus = {
  audible: { connected: true },
  hardcover: { connected: true, username: 'reader', fromEnv: false },
  sync: { running: false },
}

describe('App resolve flow', () => {
  beforeEach(() => {
    vi.spyOn(api, 'fetchPreferences').mockResolvedValue({ syncInterval: '6h0m0s', matchConfidence: 'strict', publicUrl: '' })
    window.location.hash = ''
    vi.spyOn(api, 'fetchStatus').mockResolvedValue(connectedStatus)
    vi.spyOn(api, 'fetchUnmatched').mockResolvedValue([
      { asin: 'A1', title: 'A Court of Thorns and Roses', author: 'Sarah J. Maas', status: 'finished', percentComplete: 100 },
    ])
    vi.spyOn(api, 'fetchUnmatchedDetail').mockResolvedValue({
      asin: 'A1', title: 'A Court of Thorns and Roses', author: 'Sarah J. Maas', status: 'finished', percentComplete: 100,
    })
    vi.spyOn(api, 'searchHardcover').mockResolvedValue([
      { id: 999, title: 'A Court of Thorns and Roses', authorNames: ['Sarah J. Maas'], slug: 'acotar', releaseYear: 2015 },
    ])
  })

  it('confirming a match calls resolveMatch with the right ids and returns to the shelf', async () => {
    const resolveSpy = vi.spyOn(api, 'resolveMatch').mockResolvedValue(undefined)
    const user = userEvent.setup()

    render(<App />)

    await screen.findByText('A Court of Thorns and Roses')
    await user.click(screen.getByText('A Court of Thorns and Roses'))

    const confirmButton = await screen.findByRole('button', { name: /confirm match/i })

    // After confirming, the shelf refetch should report the book resolved
    // (empty), which the assertion below checks by re-mocking fetchUnmatched.
    vi.spyOn(api, 'fetchUnmatched').mockResolvedValue([])

    await user.click(confirmButton)

    await waitFor(() => expect(resolveSpy).toHaveBeenCalledWith('A1', 999))
    expect(await screen.findByRole('status')).toHaveTextContent(/matched “A Court of Thorns and Roses”/i)
    await waitFor(() => expect(screen.getByText(/nothing to resolve/i)).toBeInTheDocument())
  })

  it('shows a visible error and keeps the panel open when resolveMatch fails', async () => {
    vi.spyOn(api, 'resolveMatch').mockRejectedValue(new Error('500 Internal Server Error'))
    const user = userEvent.setup()

    render(<App />)

    await screen.findByText('A Court of Thorns and Roses')
    await user.click(screen.getByText('A Court of Thorns and Roses'))

    const confirmButton = await screen.findByRole('button', { name: /confirm match/i })
    await user.click(confirmButton)

    // A user-friendly error message appears.
    await waitFor(() =>
      expect(screen.getByText(/couldn't confirm this match/i)).toBeInTheDocument(),
    )
    expect(screen.getByText(/500 Internal Server Error/)).toBeInTheDocument()

    // The panel stays open: the book is still selected and its resolve UI
    // (search input, confirm button) is still on screen.
    expect(screen.getByRole('button', { name: /confirm match/i })).toBeInTheDocument()
    expect(screen.getByPlaceholderText(/search hardcover/i)).toBeInTheDocument()

    // The book is still on the shelf (never removed since resolve failed) —
    // it now renders twice: once in the shelf spine, once in the still-open
    // resolve panel heading.
    expect(screen.getAllByText('A Court of Thorns and Roses').length).toBeGreaterThanOrEqual(2)
  })

  it('disables the confirm button while a confirm request is in flight', async () => {
    let resolveConfirm: () => void = () => {}
    vi.spyOn(api, 'resolveMatch').mockImplementation(
      () => new Promise((resolve) => { resolveConfirm = () => resolve(undefined) }),
    )
    const user = userEvent.setup()

    render(<App />)

    await screen.findByText('A Court of Thorns and Roses')
    await user.click(screen.getByText('A Court of Thorns and Roses'))

    const confirmButton = await screen.findByRole('button', { name: /confirm match/i })

    vi.spyOn(api, 'fetchUnmatched').mockResolvedValue([])

    await user.click(confirmButton)

    await waitFor(() => expect(confirmButton).toBeDisabled())

    resolveConfirm()

    await waitFor(() => expect(screen.getByText(/nothing to resolve/i)).toBeInTheDocument())
  })
})

describe('App navigation', () => {
  beforeEach(() => {
    vi.spyOn(api, 'fetchPreferences').mockResolvedValue({ syncInterval: '6h0m0s', matchConfidence: 'strict', publicUrl: '' })
    window.location.hash = ''
    vi.spyOn(api, 'fetchUnmatched').mockResolvedValue([])
  })

  it('opens on Setup when Audible is not connected', async () => {
    vi.spyOn(api, 'fetchStatus').mockResolvedValue({ ...connectedStatus, audible: { connected: false } })
    render(<App />)
    expect(await screen.findByRole('button', { name: /connect audible/i })).toBeInTheDocument()
  })

  it('stays on Setup after setup finishes', async () => {
    vi.spyOn(api, 'fetchStatus')
      .mockResolvedValueOnce({ ...connectedStatus, hardcover: { connected: false, fromEnv: false } })
      .mockResolvedValue(connectedStatus)
    vi.spyOn(api, 'setHardcoverToken').mockResolvedValue('reader')
    const user = userEvent.setup()
    render(<App />)
    await user.type(await screen.findByLabelText(/hardcover api token/i), 'tok')
    await user.click(screen.getByRole('button', { name: /save token/i }))
    await waitFor(() => expect(api.fetchStatus).toHaveBeenCalledTimes(2))
    expect(await screen.findByText(/@reader/)).toBeInTheDocument()
    expect(screen.queryByText(/unmatched shelf/i)).not.toBeInTheDocument()
  })

  it('opens on Matches when everything is connected', async () => {
    vi.spyOn(api, 'fetchStatus').mockResolvedValue(connectedStatus)
    render(<App />)
    expect(await screen.findByText(/unmatched shelf/i)).toBeInTheDocument()
  })
})
