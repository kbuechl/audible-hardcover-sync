import { afterEach, describe, expect, it, vi } from 'vitest'
import { resolveMatch } from './api'

describe('resolveMatch', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('succeeds when the server returns 200 with an empty body', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(null, { status: 200 })))
    await expect(resolveMatch('A1', 777)).resolves.toBeUndefined()
  })

  it('throws with the server message on a non-2xx response', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(new Response('asin and hardcoverBookId are required', { status: 400 })),
    )
    await expect(resolveMatch('', 777)).rejects.toThrow('asin and hardcoverBookId are required')
  })
})
