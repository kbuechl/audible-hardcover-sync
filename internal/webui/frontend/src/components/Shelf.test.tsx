import { render, screen } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import Shelf from './Shelf'
import type { UnmatchedBook } from '../types'

describe('Shelf', () => {
  it('renders one card per book', () => {
    const books: UnmatchedBook[] = [
      { asin: 'A1', title: 'Book One', author: 'Author One', status: 'finished', percentComplete: 100 },
      { asin: 'A2', title: 'Book Two', author: 'Author Two', status: 'in_progress', percentComplete: 40 },
    ]
    render(<Shelf books={books} selectedAsin={null} onSelect={vi.fn()} />)

    expect(screen.getByText('Book One')).toBeInTheDocument()
    expect(screen.getByText('Book Two')).toBeInTheDocument()
    expect(screen.getByText('Finished')).toBeInTheDocument()
    expect(screen.getByText('In progress')).toBeInTheDocument()
  })

  it('shows an empty state when there are no unmatched books', () => {
    render(<Shelf books={[]} selectedAsin={null} onSelect={vi.fn()} />)
    expect(screen.getByText(/nothing to resolve/i)).toBeInTheDocument()
  })

  it('calls onSelect with the clicked book\'s asin', async () => {
    const onSelect = vi.fn()
    const books: UnmatchedBook[] = [
      { asin: 'A1', title: 'Book One', author: 'Author One', status: 'finished', percentComplete: 100 },
    ]
    render(<Shelf books={books} selectedAsin={null} onSelect={onSelect} />)
    screen.getByText('Book One').closest('button')!.click()
    expect(onSelect).toHaveBeenCalledWith('A1')
  })
})
