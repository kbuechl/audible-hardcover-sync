import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import PreferencesCard from './PreferencesCard'
import * as api from '../api'

describe('PreferencesCard', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    vi.spyOn(api, 'fetchPreferences').mockResolvedValue({ syncInterval: '6h0m0s', matchConfidence: 'strict', publicUrl: '', syncMode: 'all', reviewNotify: 'after_sync' })
  })

  it('saves changed preferences', async () => {
    const save = vi.spyOn(api, 'savePreferences').mockImplementation(async (p) => p)
    const user = userEvent.setup()
    render(<PreferencesCard />)
    await user.selectOptions(await screen.findByRole('combobox'), '30m0s')
    await user.click(screen.getByLabelText('Medium'))
    await user.click(screen.getByRole('button', { name: /use http/i }))
    await user.click(screen.getByRole('button', { name: /save preferences/i }))
    await waitFor(() =>
      expect(save).toHaveBeenCalledWith({ syncInterval: '30m0s', matchConfidence: 'medium', publicUrl: window.location.origin, syncMode: 'all', reviewNotify: 'after_sync' }),
    )
    expect(await screen.findByText('Saved.')).toBeInTheDocument()
  })

  it('shows a save error', async () => {
    vi.spyOn(api, 'savePreferences').mockRejectedValue(new Error('web UI address must look like http://…'))
    const user = userEvent.setup()
    render(<PreferencesCard />)
    await user.click(await screen.findByRole('button', { name: /save preferences/i }))
    expect(await screen.findByRole('alert')).toHaveTextContent('web UI address')
  })

  it('warns that notifications have no link while the address is blank', async () => {
    render(<PreferencesCard />)
    expect(await screen.findByText(/notifications won't include a link/i)).toBeInTheDocument()
  })

  it('shows the four sync modes with the current one selected, and saves the chosen one', async () => {
    vi.spyOn(api, 'fetchPreferences').mockResolvedValue({
      syncInterval: '6h0m0s', matchConfidence: 'strict', publicUrl: '', syncMode: 'skip_shared', reviewNotify: 'daily',
    })
    const save = vi.spyOn(api, 'savePreferences').mockImplementation(async (p) => p)
    const user = userEvent.setup()
    render(<PreferencesCard />)
    expect(await screen.findByLabelText('Skip shared books')).toBeChecked()
    for (const l of ['Sync everything', 'Confirm shared books', 'Confirm every book']) {
      expect(screen.getByLabelText(l)).not.toBeChecked()
    }
    expect(screen.getByText('Books shared from another Audible account are left out.')).toBeInTheDocument()
    await user.click(screen.getByLabelText('Confirm every book'))
    expect(screen.getByText('Every new book waits on the Review page until you choose.')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /save preferences/i }))
    await waitFor(() =>
      expect(save).toHaveBeenCalledWith({
        syncInterval: '6h0m0s', matchConfidence: 'strict', publicUrl: '', syncMode: 'confirm_all', reviewNotify: 'daily',
      }),
    )
  })
})
