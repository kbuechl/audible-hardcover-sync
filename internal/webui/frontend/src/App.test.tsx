import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import App from './App'
import * as api from './api'

describe('App resolve flow', () => {
  beforeEach(() => {
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
    await waitFor(() => expect(screen.getByText(/nothing to resolve/i)).toBeInTheDocument())
  })
})
