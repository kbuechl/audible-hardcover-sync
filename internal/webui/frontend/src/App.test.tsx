import { render, screen, waitFor } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import App from './App'

beforeEach(() => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, json: async () => [] }))
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('App', () => {
  it('renders the page heading', () => {
    render(<App />)
    expect(screen.getByText('Unmatched Shelf')).toBeInTheDocument()
  })

  it('fetches unmatched books on mount and shows the empty state when there are none', async () => {
    render(<App />)

    await waitFor(() => {
      expect(screen.getByText(/nothing to resolve/i)).toBeInTheDocument()
    })

    expect(fetch).toHaveBeenCalledWith('/api/unmatched', undefined)
    expect(screen.getByText('0')).toBeInTheDocument()
  })

  it('renders a shelf card for each fetched book', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: true,
        json: async () => [
          { asin: 'A1', title: 'Book One', author: 'Author One', status: 'finished', percentComplete: 100 },
        ],
      }),
    )

    render(<App />)

    await waitFor(() => {
      expect(screen.getByText('Book One')).toBeInTheDocument()
    })
    expect(screen.getByText('Select a book to resolve it.')).toBeInTheDocument()
  })
})
