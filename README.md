# audible-hardcover-sync

Syncs Audible listening history to Hardcover: finished books → Read,
in-progress books → Currently Reading. Audible has no public API, so this
uses a vendored, extended fork of the reverse-engineered `go-audible`
client. Matching is ISBN/ASIN-exact first, then fuzzy title/author;
books that can't be matched automatically show up on a manual-match
web page, with optional notifications (Slack/Discord/email/etc via
shoutrrr) when that happens.

See `docs/superpowers/specs/2026-07-23-audible-hardcover-sync-design.md`
for the full design.

## Setup

1. Get a Hardcover API token from your account settings page.
2. Copy `.env.example` to `.env` and fill in `HARDCOVER_API_TOKEN`,
   `PUBLIC_URL` (e.g. `http://192.168.1.10:8080`), and optionally
   `NOTIFY_URLS` (comma-separated shoutrrr service URLs).
3. One-time interactive Audible login:
   ```bash
   docker compose run --rm sync-tool auth
   ```
   Visit the printed URL, sign in, paste the redirect URL back.
4. Start the service:
   ```bash
   docker compose up -d
   ```
5. Unmatched books (no ISBN, or no Hardcover match found) appear at
   `${PUBLIC_URL}/` — search and confirm the right Hardcover
   edition there.

## Security note

The manual-match web UI has no authentication. It's meant for a LAN/VPN
trust boundary only — don't port-forward `WEB_UI_PORT` to the internet.

## Third-party licenses

See [THIRD_PARTY_LICENSES.md](THIRD_PARTY_LICENSES.md) for a full list of
vendored source and dependencies with their licenses.

## Sync behavior

- **Matching priority.** The sync matches on ISBN/ASIN exactly first, then
  falls back to a confidence-scored fuzzy title/author match. The fuzzy
  threshold is configurable via the `MATCH_CONFIDENCE` env var: `strict`
  (0.95, default), `medium` (0.75), `loose` (0.55), or a raw
  number/percentage.
- **Existing entries are skipped.** If a book is already in your Hardcover
  library (any status — Read, Currently Reading, Want to Read), the sync
  will NOT modify it:
  - A book you read in print and marked Read will NOT be downgraded to
    Currently Reading when you start the audiobook on Audible.
  - A rating you set on Hardcover will NOT be overwritten by your Audible
    rating.
  - A finish date you set manually on Hardcover will NOT be overwritten.
  - The one exception is listening progress: for a book that is Currently
    Reading on Hardcover, progress is moved forward to match Audible on each
    sync. It is never lowered, and nothing else about the entry changes.
- **New entries get.** When a book is not already on Hardcover, the sync
  inserts it with:
  - Status (Read for finished, Currently Reading for in-progress).
  - The audiobook edition (matched by Audible ASIN, falling back to any
    edition with audio duration).
  - The finish date from Audible's listening history (finished books only).
  - Your Audible star rating (if you've rated the book on Audible).
  - Listening progress (in-progress books), from Audible's percent complete
    and the audiobook's runtime.
- **Rate limiting.** Throttled requests (HTTP 429) are retried with
  exponential backoff for read operations (searches, lookups), but
  mutations (inserts/updates) are NOT retried to prevent duplicate
  entries.
