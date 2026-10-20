import type { SearchResultItem } from './types'

const normalize = (s: string) => s.trim().toLowerCase()
// nameKey reduces a name to first + last word, so "Ben H. Winters" and
// "Ben Winters" agree but "Elizabeth Bernstein" and "Gabrielle Bernstein" don't.
function nameKey(name: string): string {
  const words = normalize(name).replace(/[.,]/g, ' ').split(/\s+/).filter(Boolean)
  return words.length === 0 ? '' : `${words[0]} ${words.at(-1)}`
}

function nameKeys(names: string): Set<string> {
  return new Set(names.split(/,|&| and /).map(nameKey).filter(Boolean))
}

// matchScore rates how well a Hardcover result fits the Audible book:
// title (exact 4, partial 1) plus 2 when an author's first and last name match.
export function matchScore(result: SearchResultItem, title: string, author = ''): number {
  const want = normalize(title)
  const got = normalize(result.title)
  let score = got === want ? 4 : got.includes(want) || want.includes(got) ? 1 : 0
  const authors = nameKeys(author)
  if (authors.size > 0 && result.authorNames.some((a) => authors.has(nameKey(a)))) score += 2
  return score
}

// rankResults orders results best first, keeping Hardcover's order for ties.
// bestFound is true when the top result is a confident match: exact title
// and, when Audible lists an author, a matching author.
export function rankResults(
  results: SearchResultItem[],
  title: string,
  author = '',
): { ranked: SearchResultItem[]; bestFound: boolean } {
  const scored = results.map((r, i) => ({ r, i, s: matchScore(r, title, author) }))
  scored.sort((a, b) => b.s - a.s || a.i - b.i)
  const confident = nameKeys(author).size > 0 ? 6 : 4
  return { ranked: scored.map((x) => x.r), bestFound: scored.length > 0 && scored[0].s >= confident }
}
