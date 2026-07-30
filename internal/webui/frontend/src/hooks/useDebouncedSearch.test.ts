import { renderHook, waitFor, act } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { useDebouncedSearch } from './useDebouncedSearch'
import * as api from '../api'

describe('useDebouncedSearch', () => {
  beforeEach(() => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
  })

  it('fires an initial search immediately using the prefilled query', async () => {
    const spy = vi.spyOn(api, 'searchHardcover').mockResolvedValue([
      { id: 1, title: 'Match', authorNames: ['Someone'], slug: 'match' },
    ])
    const { result } = renderHook(() => useDebouncedSearch('A1', 'Prefilled Title'))

    await waitFor(() => expect(spy).toHaveBeenCalledWith('A1', 'Prefilled Title'))
    await waitFor(() => expect(result.current.results).toHaveLength(1))
    expect(result.current.query).toBe('Prefilled Title')
  })

  it('debounces subsequent edits by 300ms rather than firing on every keystroke', async () => {
    const spy = vi.spyOn(api, 'searchHardcover').mockResolvedValue([])
    const { result } = renderHook(() => useDebouncedSearch('A1', 'Initial'))
    await waitFor(() => expect(spy).toHaveBeenCalledTimes(1))

    act(() => result.current.setQuery('I'))
    act(() => result.current.setQuery('In'))
    act(() => result.current.setQuery('Int'))

    // Not enough time has passed — still just the initial call.
    expect(spy).toHaveBeenCalledTimes(1)

    act(() => vi.advanceTimersByTime(300))
    await waitFor(() => expect(spy).toHaveBeenCalledTimes(2))
    expect(spy).toHaveBeenLastCalledWith('A1', 'Int')
  })

  it('does not search when asin is null', () => {
    const spy = vi.spyOn(api, 'searchHardcover').mockResolvedValue([])
    renderHook(() => useDebouncedSearch(null, 'Something'))
    expect(spy).not.toHaveBeenCalled()
  })

  it('surfaces a loading state while a search is in flight', async () => {
    let resolveSearch: (v: never[]) => void = () => {}
    const spy = vi.spyOn(api, 'searchHardcover').mockImplementation(
      () => new Promise((resolve) => { resolveSearch = resolve }),
    )
    const { result } = renderHook(() => useDebouncedSearch('A1', 'Initial'))

    await waitFor(() => expect(result.current.loading).toBe(true))
    act(() => resolveSearch([]))
    await waitFor(() => expect(result.current.loading).toBe(false))
    expect(spy).toHaveBeenCalled()
  })

  it('re-fires immediately with the NEW prefilled query when the selected book changes mid-session — not the stale previous-book query', async () => {
    // Regression test: a naive two-effect implementation can fire the
    // "immediate on first run" search using the *previous* book's query
    // (still in the `query` state at the moment the effect runs) instead of
    // the newly-selected book's prefilled title, because effects in the
    // same commit see the pre-update state. This must never search with
    // book A's leftover query text after the user has moved on to book B.
    const spy = vi.spyOn(api, 'searchHardcover').mockResolvedValue([])
    const { result, rerender } = renderHook(
      ({ asin, initialQuery }) => useDebouncedSearch(asin, initialQuery),
      { initialProps: { asin: 'A1', initialQuery: 'Book A Title' } },
    )
    await waitFor(() => expect(spy).toHaveBeenCalledWith('A1', 'Book A Title'))

    // User edits the search box for book A before switching away.
    act(() => result.current.setQuery('Book A Title Edited'))

    // User selects a different book before the 300ms debounce for the edit
    // above has elapsed.
    rerender({ asin: 'A2', initialQuery: 'Book B Title' })

    await waitFor(() => expect(result.current.query).toBe('Book B Title'))
    await waitFor(() => expect(spy).toHaveBeenCalledWith('A2', 'Book B Title'))
    // The critical assertion: no call was ever made for A2 carrying book
    // A's stale/edited query text.
    for (const call of spy.mock.calls) {
      if (call[0] === 'A2') {
        expect(call[1]).toBe('Book B Title')
      }
    }
  })
})
