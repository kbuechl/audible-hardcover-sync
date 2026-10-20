import { describe, it, expect } from 'vitest'
import { rankResults } from './match'

const r = (id: number, title: string, ...authorNames: string[]) => ({ id, title, authorNames, slug: '' })
const ids = (x: { ranked: { id: number }[] }) => x.ranked.map((b) => b.id)

describe('rankResults', () => {
  it('puts the title and author match first', () => {
    const got = rankResults(
      [r(1, 'Hardcore Self Help', 'Robert Duff'), r(2, 'Self Help', 'Ben H. Winters'), r(3, ' self help ', 'Gabrielle Bernstein')],
      'Self Help',
      'Gabrielle Bernstein',
    )
    expect(ids(got)).toEqual([3, 2, 1])
    expect(got.bestFound).toBe(true)
  })
  it('keeps Hardcover order for ties and flags no best without an author match', () => {
    const got = rankResults([r(1, 'Self Help', 'A Writer'), r(2, 'Self Help', 'B Writer')], 'Self Help', 'Gabrielle Bernstein')
    expect(ids(got)).toEqual([1, 2])
    expect(got.bestFound).toBe(false)
  })
  it('does not match on a shared last name alone', () => {
    const got = rankResults([r(1, 'Self Help', 'Elizabeth Bernstein', 'Gabrielle Gilbert')], 'Self Help', 'Gabrielle Bernstein')
    expect(got.bestFound).toBe(false)
  })
  it('ignores middle initials', () => {
    expect(rankResults([r(1, 'Self Help', 'Ben H. Winters')], 'Self Help', 'Ben Winters').bestFound).toBe(true)
  })
  it('matches any of several Audible authors', () => {
    expect(rankResults([r(1, 'Good Omens', 'Neil Gaiman')], 'Good Omens', 'Terry Pratchett, Neil Gaiman').bestFound).toBe(true)
  })
  it('uses the title alone when Audible has no author', () => {
    const got = rankResults([r(1, 'Dune Messiah'), r(2, 'Dune')], 'Dune')
    expect(ids(got)).toEqual([2, 1])
    expect(got.bestFound).toBe(true)
  })
  it('handles no results', () => {
    expect(rankResults([], 'Dune').bestFound).toBe(false)
  })
})
