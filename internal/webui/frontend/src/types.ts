export interface UnmatchedBook {
  asin: string
  title: string
  author: string
  status: string
  percentComplete: number
}

export interface SearchResultItem {
  id: number
  title: string
  authorNames: string[]
  releaseYear?: number
  slug: string
  coverUrl?: string
}
