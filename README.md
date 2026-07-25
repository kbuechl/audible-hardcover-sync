# audible-hardcover-sync

Syncs Audible listening history to Hardcover: finished books → Read,
in-progress books → Currently Reading. Audible has no public API, so this
uses a vendored, extended fork of the reverse-engineered `go-audible`
client. Matching is ISBN-exact only; books that can't be matched
automatically show up on a manual-match web page, with optional
notifications (Slack/Discord/email/etc via shoutrrr) when that happens.

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
   `${PUBLIC_URL}/unmatched` — search and confirm the right Hardcover
   edition there.

## Security note

The manual-match web UI has no authentication. It's meant for a LAN/VPN
trust boundary only — don't port-forward `WEB_UI_PORT` to the internet.
