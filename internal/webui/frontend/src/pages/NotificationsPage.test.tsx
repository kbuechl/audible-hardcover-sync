import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import NotificationsPage from './NotificationsPage'
import * as api from '../api'

describe('NotificationsPage', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    vi.spyOn(api, 'fetchNotifyTargets').mockResolvedValue([
      { id: 0, masked: 'discord://****@123' },
      { id: 1, masked: 'ntfy://ntfy.sh/bo****' },
    ])
  })

  it('lists masked targets, each removable', async () => {
    render(<NotificationsPage />)
    expect(await screen.findByText('discord://****@123')).toBeInTheDocument()
    expect(screen.getAllByRole('button', { name: /remove/i })).toHaveLength(2)
  })

  it('shows a failed test inline', async () => {
    vi.spyOn(api, 'testNotifyTarget').mockRejectedValue(new Error('401 from discord'))
    const user = userEvent.setup()
    render(<NotificationsPage />)
    await screen.findByText('discord://****@123')
    await user.click(screen.getAllByRole('button', { name: /send test/i })[0])
    expect(await screen.findByRole('alert')).toHaveTextContent('401 from discord')
  })

  it('tests and saves a new URL', async () => {
    const test = vi.spyOn(api, 'testNotifyURL').mockResolvedValue(undefined)
    const add = vi.spyOn(api, 'addNotifyURL').mockResolvedValue(undefined)
    const user = userEvent.setup()
    render(<NotificationsPage />)
    await user.type(screen.getByLabelText(/notification url/i), 'ntfy://ntfy.sh/x')
    await user.click(screen.getAllByRole('button', { name: /send test/i }).at(-1)!)
    await waitFor(() => expect(test).toHaveBeenCalledWith('ntfy://ntfy.sh/x'))
    await user.click(screen.getByRole('button', { name: /save/i }))
    await waitFor(() => expect(add).toHaveBeenCalledWith('ntfy://ntfy.sh/x'))
  })
})
