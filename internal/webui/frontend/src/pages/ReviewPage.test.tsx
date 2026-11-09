import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import ReviewPage from './ReviewPage'
import * as api from '../api'
import type { ReviewItem } from '../types'

const base: ReviewItem = {
  asin: 'A1', title: 'Dune', author: 'Frank Herbert', shared: false, status: 'Listening',
  percentComplete: 40, decision: '', canRemove: false,
}
const waiting: ReviewItem[] = [
  base,
  { ...base, asin: 'A2', title: 'Emma', author: 'Jane Austen', shared: true },
]
const synced: ReviewItem[] = [
  { ...base, asin: 'S1', title: 'Hyperion', decision: 'sync', canRemove: true },
  { ...base, asin: 'S2', title: 'Old Book', decision: 'sync', canRemove: false },
]
const notWanted: ReviewItem[] = [{ ...base, asin: 'N1', title: 'Skipped', decision: 'not_wanted' }]

function mockList() {
  return vi.spyOn(api, 'fetchReview').mockImplementation(async (filter, q) => {
    const all = filter === 'waiting' ? waiting : filter === 'synced' ? synced : notWanted
    return all.filter((i) => i.title.toLowerCase().includes(q.toLowerCase()))
  })
}

describe('ReviewPage', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
  })

  it('switches the list with the filter tabs', async () => {
    mockList()
    const user = userEvent.setup()
    render(<ReviewPage />)
    expect(await screen.findByText('Dune')).toBeInTheDocument()
    await user.click(screen.getByRole('tab', { name: 'Not wanted' }))
    expect(await screen.findByText('Skipped')).toBeInTheDocument()
    expect(screen.queryByText('Dune')).not.toBeInTheDocument()
  })

  it('searches titles', async () => {
    const list = mockList()
    const user = userEvent.setup()
    render(<ReviewPage />)
    await screen.findByText('Dune')
    await user.type(screen.getByLabelText('Search titles'), 'emma')
    await waitFor(() => expect(list).toHaveBeenLastCalledWith('waiting', 'emma'))
    expect(await screen.findByText('Emma')).toBeInTheDocument()
    expect(screen.queryByText('Dune')).not.toBeInTheDocument()
  })

  it('select-all + Sync sends every visible asin, toasts and refreshes', async () => {
    mockList()
    const decide = vi.spyOn(api, 'setReviewDecision').mockResolvedValue(undefined)
    const changed = vi.fn()
    const user = userEvent.setup()
    render(<ReviewPage onChanged={changed} />)
    await screen.findByText('Dune')
    await user.click(screen.getByLabelText(/select all/i))
    await user.click(screen.getByRole('button', { name: 'Sync selected' }))
    expect(decide).toHaveBeenCalledWith(['A1', 'A2'], 'sync')
    expect(await screen.findByRole('status')).toHaveTextContent('2 books')
    expect(changed).toHaveBeenCalled()
  })

  it('shows the Shared badge only on shared books', async () => {
    mockList()
    render(<ReviewPage />)
    const emma = (await screen.findByText('Emma')).closest('li')!
    expect(within(emma).getByText('Shared')).toBeInTheDocument()
    const dune = screen.getByText('Dune').closest('li')!
    expect(within(dune).queryByText('Shared')).not.toBeInTheDocument()
  })

  it('removes only after the dialog is confirmed, listing what else is deleted', async () => {
    mockList()
    vi.spyOn(api, 'previewRemove').mockResolvedValue({
      deletes: ['Your library entry'],
      alsoDeletes: ['Your 4-star rating'],
    })
    const remove = vi.spyOn(api, 'removeFromHardcover').mockResolvedValue(undefined)
    const user = userEvent.setup()
    render(<ReviewPage />)
    await user.click(await screen.findByRole('tab', { name: 'Synced' }))
    await screen.findByText('Hyperion')
    await user.click(screen.getByRole('button', { name: 'Remove from Hardcover' }))
    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByText('Your 4-star rating')).toBeInTheDocument()
    expect(within(dialog).getByText(/also deletes/i)).toBeInTheDocument()
    expect(remove).not.toHaveBeenCalled()
    await user.click(within(dialog).getByRole('button', { name: 'Remove from Hardcover' }))
    expect(remove).toHaveBeenCalledWith('S1')
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  })

  it('keeps the book when the dialog is cancelled', async () => {
    mockList()
    vi.spyOn(api, 'previewRemove').mockResolvedValue({ deletes: ['Your library entry'], alsoDeletes: [] })
    const remove = vi.spyOn(api, 'removeFromHardcover').mockResolvedValue(undefined)
    const user = userEvent.setup()
    render(<ReviewPage />)
    await user.click(await screen.findByRole('tab', { name: 'Synced' }))
    await screen.findByText('Hyperion')
    await user.click(screen.getByRole('button', { name: 'Remove from Hardcover' }))
    await user.click(await screen.findByRole('button', { name: 'Keep it' }))
    expect(remove).not.toHaveBeenCalled()
  })

  it('hides Remove when canRemove is false', async () => {
    mockList()
    const user = userEvent.setup()
    render(<ReviewPage />)
    await user.click(await screen.findByRole('tab', { name: 'Synced' }))
    const old = (await screen.findByText('Old Book')).closest('li')!
    expect(within(old).queryByRole('button', { name: /remove/i })).not.toBeInTheDocument()
    expect(within(old).getByRole('button', { name: 'Not wanted' })).toBeInTheDocument()
  })

  it('shows an all-caught-up state when nothing is waiting', async () => {
    vi.spyOn(api, 'fetchReview').mockResolvedValue([])
    render(<ReviewPage />)
    expect(await screen.findByText('All caught up')).toBeInTheDocument()
  })

  it('shows the newer response when an older request resolves last', async () => {
    let resolveOld: (v: ReviewItem[]) => void = () => {}
    const list = vi.spyOn(api, 'fetchReview')
    list.mockImplementationOnce(() => new Promise((r) => { resolveOld = r }))
    list.mockImplementationOnce(async () => notWanted)
    const user = userEvent.setup()
    render(<ReviewPage />)
    await user.click(screen.getByRole('tab', { name: 'Not wanted' }))
    expect(await screen.findByText('Skipped')).toBeInTheDocument()
    resolveOld(waiting)
    await new Promise((r) => setTimeout(r, 20))
    expect(screen.getByText('Skipped')).toBeInTheDocument()
    expect(screen.queryByText('Dune')).not.toBeInTheDocument()
  })

  it('shows the error, not an empty state, when loading fails', async () => {
    vi.spyOn(api, 'fetchReview').mockRejectedValue(new Error('boom'))
    render(<ReviewPage />)
    expect(await screen.findByRole('alert')).toHaveTextContent('boom')
    expect(screen.queryByText('All caught up')).not.toBeInTheDocument()
  })

  it('bulk Not wanted sends the selected asins', async () => {
    mockList()
    const decide = vi.spyOn(api, 'setReviewDecision').mockResolvedValue(undefined)
    const user = userEvent.setup()
    render(<ReviewPage />)
    await screen.findByText('Dune')
    await user.click(screen.getByLabelText('Select Emma'))
    await user.click(screen.getByRole('button', { name: 'Not wanted selected' }))
    expect(decide).toHaveBeenCalledWith(['A2'], 'not_wanted')
    expect(await screen.findByRole('status')).toHaveTextContent('1 book marked Not wanted')
  })

  it('toasts and refreshes after a confirmed remove; Escape cancels the dialog', async () => {
    mockList()
    vi.spyOn(api, 'previewRemove').mockResolvedValue({ deletes: ['Your library entry'], alsoDeletes: [] })
    vi.spyOn(api, 'removeFromHardcover').mockResolvedValue(undefined)
    const changed = vi.fn()
    const user = userEvent.setup()
    render(<ReviewPage onChanged={changed} />)
    await user.click(await screen.findByRole('tab', { name: 'Synced' }))
    await screen.findByText('Hyperion')
    await user.click(screen.getByRole('button', { name: 'Remove from Hardcover' }))
    await screen.findByRole('dialog')
    await user.keyboard('{Escape}')
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Remove from Hardcover' }))
    await user.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Remove from Hardcover' }))
    expect(await screen.findByRole('status')).toHaveTextContent('Removed “Hyperion”')
    expect(changed).toHaveBeenCalled()
  })
})
