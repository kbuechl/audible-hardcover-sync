import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import SetupPage from './SetupPage'
import * as api from '../api'
import type { Status } from '../types'

const base: Status = {
  audible: { connected: false },
  hardcover: { connected: false, fromEnv: false },
  sync: { running: false },
}

describe('SetupPage', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    vi.spyOn(api, 'fetchPreferences').mockResolvedValue({ syncInterval: '6h0m0s', matchConfidence: 'strict', publicUrl: '' })
    vi.spyOn(window, 'open').mockReturnValue(null)
  })

  it('connects Audible with the pasted address', async () => {
    vi.spyOn(api, 'startAudibleLogin').mockResolvedValue('https://amazon.example/signin')
    const finish = vi.spyOn(api, 'finishAudibleLogin').mockResolvedValue(undefined)
    const onChanged = vi.fn()
    const user = userEvent.setup()
    render(<SetupPage status={base} onChanged={onChanged} />)

    await user.click(screen.getByRole('button', { name: /connect audible/i }))
    await user.type(await screen.findByLabelText(/address after signing in/i), 'https://www.amazon.com/ap/maplanding?code=x')
    await user.click(screen.getByRole('button', { name: /finish connecting/i }))

    await waitFor(() => expect(finish).toHaveBeenCalledWith('https://www.amazon.com/ap/maplanding?code=x'))
    expect(onChanged).toHaveBeenCalled()
  })

  it('shows the error when Hardcover rejects the token, and never shows a saved token', async () => {
    vi.spyOn(api, 'setHardcoverToken').mockRejectedValue(new Error("Hardcover didn't accept that token"))
    const user = userEvent.setup()
    render(<SetupPage status={base} onChanged={vi.fn()} />)

    const input = screen.getByLabelText(/hardcover api token/i)
    expect(input).toHaveAttribute('type', 'password')
    expect(input).toHaveValue('')
    await user.type(input, 'bad')
    await user.click(screen.getByRole('button', { name: /save token/i }))
    expect(await screen.findByRole('alert')).toHaveTextContent(/didn't accept/)
  })

  it('locks the token form when the token comes from the environment', () => {
    render(<SetupPage status={{ ...base, hardcover: { connected: true, username: 'reader', fromEnv: true } }} onChanged={vi.fn()} />)
    expect(screen.queryByLabelText(/hardcover api token/i)).not.toBeInTheDocument()
    expect(screen.getByText(/HARDCOVER_API_TOKEN/)).toBeInTheDocument()
  })

  it('disables Sync now until both services are connected', () => {
    render(<SetupPage status={base} onChanged={vi.fn()} />)
    expect(screen.getByRole('button', { name: /sync now/i })).toBeDisabled()
  })
})
