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
  waiting?: number
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
  syncMode: 'all' | 'skip_shared' | 'confirm_shared' | 'confirm_all'
  reviewNotify: 'after_sync' | 'daily' | 'weekly' | 'off'
}

export type ReviewFilter = 'waiting' | 'synced' | 'not_wanted'
export type ReviewDecision = 'sync' | 'not_wanted' | ''

export interface ReviewItem {
  asin: string
  title: string
  author: string
  coverUrl?: string
  shared: boolean
  status: string
  percentComplete: number
  decision: ReviewDecision
  canRemove: boolean
}

export interface RemovePreview {
  deletes: string[]
  alsoDeletes: string[]
}
