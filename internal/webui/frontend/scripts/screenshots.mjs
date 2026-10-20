// Regenerates the README screenshots from the built UI with made-up data:
// public-domain books, a fake account and placeholder notification URLs.
// No server, Audible or Hardcover account is involved.
//
//   npm run build && npm run screenshots
//
// Needs a Chromium for Playwright (`npx playwright install chromium`), or
// set CHROMIUM_PATH to an existing one.
import { mkdir } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import { preview } from 'vite'
import { chromium, devices } from 'playwright'

const outDir = fileURLToPath(new URL('../../../../assets/screenshots/', import.meta.url))
const minutesAgo = (m) => new Date(Date.now() - m * 60_000).toISOString()

const status = {
  audible: { connected: true },
  hardcover: { connected: true, username: 'reader', fromEnv: false },
  sync: { running: false, last: { finishedAt: minutesAgo(12), synced: 3, unchanged: 241, unmatched: 3, errored: 0 } },
}

const unmatched = [
  { asin: 'B0DEMO0001', title: 'Moby-Dick; or, The Whale', author: 'Herman Melville', status: 'in_progress', percentComplete: 42 },
  { asin: 'B0DEMO0002', title: 'The Count of Monte Cristo', author: 'Alexandre Dumas', status: 'finished', percentComplete: 100 },
  { asin: 'B0DEMO0003', title: 'Middlemarch', author: 'George Eliot', status: 'finished', percentComplete: 100 },
]

const search = [
  { id: 1, title: 'Moby-Dick', authorNames: ['Herman Melville'], slug: 'moby-dick', releaseYear: 1851 },
  { id: 2, title: 'Moby-Dick; or, The Whale', authorNames: ['Herman Melville'], slug: 'moby-dick-or-the-whale', releaseYear: 1851 },
  { id: 3, title: 'Moby-Dick (Graphic Novel)', authorNames: ['Lance Stahlberg'], slug: 'moby-dick-graphic', releaseYear: 2010 },
  { id: 4, title: 'Why Read Moby-Dick?', authorNames: ['Nathaniel Philbrick'], slug: 'why-read-moby-dick', releaseYear: 2011 },
]

const notifications = [
  { id: 0, masked: 'ntfy://ntfy.sh/my-audiobooks' },
  { id: 1, masked: 'gotify://gotify.home.lan/' },
]

const preferences = { syncInterval: '30m0s', matchConfidence: 'strict', publicUrl: 'http://192.168.1.10:8080' }

async function mockAPI(page) {
  await page.route('**/api/**', (route) => {
    const path = new URL(route.request().url()).pathname
    if (path === '/api/status') return route.fulfill({ json: status })
    if (path === '/api/unmatched') return route.fulfill({ json: unmatched })
    if (path.startsWith('/api/unmatched/')) {
      return route.fulfill({ json: unmatched.find((b) => path.endsWith(b.asin)) })
    }
    if (path === '/api/search') return route.fulfill({ json: search })
    if (path === '/api/notifications') return route.fulfill({ json: notifications })
    if (path === '/api/preferences') return route.fulfill({ json: preferences })
    return route.fulfill({ status: 204 })
  })
}

const shots = [
  { name: 'matches', hash: 'matches', theme: 'light', size: 'desktop', open: 'Moby-Dick; or, The Whale' },
  { name: 'setup', hash: 'setup', theme: 'dark', size: 'desktop', fullPage: true },
  { name: 'notifications', hash: 'notifications', theme: 'light', size: 'desktop' },
  { name: 'phone-matches', hash: 'matches', theme: 'dark', size: 'phone', open: 'Moby-Dick; or, The Whale', scrollTo: 'Resolve match' },
  { name: 'phone-setup', hash: 'setup', theme: 'light', size: 'phone' },
]

const sizes = {
  desktop: { viewport: { width: 1200, height: 820 }, deviceScaleFactor: 2 },
  phone: devices['Pixel 7'],
}

const server = await preview({ preview: { port: 4174, strictPort: true }, logLevel: 'warn' })
const browser = await chromium.launch({ executablePath: process.env.CHROMIUM_PATH || undefined })
await mkdir(outDir, { recursive: true })
try {
  for (const shot of shots) {
    const page = await browser.newPage({ ...sizes[shot.size], colorScheme: shot.theme })
    await mockAPI(page)
    await page.goto(`http://localhost:4174/#${shot.hash}`)
    await page.waitForLoadState('networkidle')
    if (shot.open) {
      await page.getByText(shot.open).first().click()
      await page.getByText('Best match').waitFor()
    }
    if (shot.scrollTo) await page.getByText(shot.scrollTo).scrollIntoViewIfNeeded()
    await page.waitForTimeout(400) // let the tab fade finish
    await page.screenshot({ path: `${outDir}${shot.name}.png`, fullPage: shot.fullPage ?? false })
    await page.close()
    console.log(`wrote assets/screenshots/${shot.name}.png`)
  }
} finally {
  await browser.close()
  server.httpServer.close()
}
