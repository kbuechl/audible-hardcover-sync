import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import NotificationsPage from './NotificationsPage'
import * as api from '../api'
import type { Preferences } from '../types'

const prefs: Preferences = {
  syncInterval: '6h0m0s', matchConfidence: 'medium', publicUrl: 'http://x', syncMode: 'confirm_all', reviewNotify: 'weekly',
}

describe('NotificationsPage', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    vi.spyOn(api, 'fetchNotifyTargets').mockResolvedValue([
      { id: 0, masked: 'discord://****@123' },
      { id: 1, masked: 'ntfy://ntfy.sh/bo****' },
    ])
    vi.spyOn(api, 'fetchPreferences').mockResolvedValue(prefs)
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

  it('shows the review message choice and saves it without clobbering other preferences', async () => {
    const save = vi.spyOn(api, 'savePreferences').mockImplementation(async (p) => p)
    const user = userEvent.setup()
    render(<NotificationsPage />)
    expect(await screen.findByText('Books waiting for review')).toBeInTheDocument()
    const select = await screen.findByLabelText('Review message')
    await waitFor(() => expect(select).toHaveValue('weekly'))
    expect(screen.getAllByRole('option').map((o) => o.textContent)).toEqual(['After each sync', 'Once a day', 'Once a week', 'Off'])
    await user.selectOptions(select, 'daily')
    await waitFor(() => expect(save).toHaveBeenCalledWith({ ...prefs, reviewNotify: 'daily' }))
    expect(await screen.findByText('Saved.')).toBeInTheDocument()
  })
})
