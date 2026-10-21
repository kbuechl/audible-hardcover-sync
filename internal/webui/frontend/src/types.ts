export interface UnmatchedBook {
  asin: string
  title: string
  author: string
  status: string
  percentComplete: number
  coverUrl?: string
}

export interface SearchResultItem {
  id: number
  title: string
  authorNames: string[]
  releaseYear?: number
  slug: string
  coverUrl?: string
}

export interface SyncReport {
  finishedAt: string
  synced: number
  unchanged: number
  unmatched: number
  errored: number
  error?: string
}

export interface Status {
  audible: { connected: boolean; error?: string }
  hardcover: { connected: boolean; username?: string; fromEnv: boolean; error?: string }
  sync: { running: boolean; last?: SyncReport }
  version?: string
}

export interface NotifyTarget {
  id: number
  masked: string
}

export interface Preferences {
  syncInterval: string
  matchConfidence: 'strict' | 'medium' | 'loose'
  publicUrl: string
}
