import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import PreferencesCard from './PreferencesCard'
import * as api from '../api'

describe('PreferencesCard', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    vi.spyOn(api, 'fetchPreferences').mockResolvedValue({ syncInterval: '6h0m0s', matchConfidence: 'strict', publicUrl: '' })
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
      expect(save).toHaveBeenCalledWith({ syncInterval: '30m0s', matchConfidence: 'medium', publicUrl: window.location.origin }),
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
})
