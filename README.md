# audible-hardcover-sync

Syncs Audible listening history to Hardcover: finished books → Read,
in-progress books → Currently Reading. Audible has no public API, so this
uses a vendored, extended fork of the reverse-engineered `go-audible`
client. Matching is ISBN/ASIN-exact first, then fuzzy title/author;
books that can't be matched automatically show up on a manual-match
web page, with optional notifications (Slack/Discord/email/etc via
shoutrrr) when that happens.

> [!WARNING]
> **The web UI has no login.** Anyone who can reach it can see your
> library and change matches that are written to your Hardcover account.
> Run it on your home network or behind a VPN only. Don't port-forward it,
> and don't put it behind a public reverse proxy without adding
> authentication in front of it.

## Requirements

- Docker with the Compose plugin (amd64 or arm64).
- An Audible account on the **US** marketplace (audible.com). Other
  marketplaces aren't supported yet.
- A Hardcover account and API token: hardcover.app → Settings → API.

## Quick start

1. Get the compose file and env template:
   ```bash
   mkdir audible-hardcover-sync && cd audible-hardcover-sync
   curl -fsSLO https://raw.githubusercontent.com/kbuechl/audible-hardcover-sync/master/docker-compose.yml
   curl -fsSL https://raw.githubusercontent.com/kbuechl/audible-hardcover-sync/master/.env.example -o .env
   ```
2. Edit `.env`: set `HARDCOVER_API_TOKEN`, and `PUBLIC_URL` if you use
   notifications (see [Configuration](#configuration)).
3. Pull the image and log in to Audible once:
   ```bash
   docker compose pull
   docker compose run --rm sync-tool auth
   ```
   Open the printed URL, sign in to Amazon, then copy the URL of the page
   you land on (it may look like an error page) and paste it back into the
   terminal. Credentials are saved to the `sync-data` volume.
4. Start it:
   ```bash
   docker compose up -d
   docker compose logs -f
   ```
   The first sync starts right away. Wait for
   `sync complete synced=… unmatched=…`.
5. Open the web UI (`http://<host>:8080`) to match any books it couldn't
   match on its own.

The first sync of a large library takes a while: Hardcover's free API
tier allows about 60 requests a minute, and the tool paces itself to
that.

### Already running Compose stacks?

Add the service to an existing compose file instead:

```yaml
  audible-hardcover-sync:
    image: ghcr.io/kbuechl/audible-hardcover-sync:latest
    restart: unless-stopped
    environment:
      - HARDCOVER_API_TOKEN
      - PUBLIC_URL
      - NOTIFY_URLS
      - SYNC_INTERVAL=30m
    volumes:
      - ./audible-hardcover-sync:/data
    ports:
      # No login on the web UI: LAN only, never exposed to the internet.
      - 8080:8080
```

Then run `docker compose run --rm audible-hardcover-sync auth` once
before `docker compose up -d audible-hardcover-sync`.

## Configuration

Set in `.env` (or the service's `environment`).

| Variable | Required | Default | What it does |
|---|---|---|---|
| `HARDCOVER_API_TOKEN` | yes | | Your Hardcover API token. |
| `PUBLIC_URL` | no | | Address of the web UI, used for links in notifications, e.g. `http://192.168.1.10:8080`. Use a LAN or VPN address: the UI has no login, so it must not be publicly reachable. |
| `SYNC_INTERVAL` | no | `6h` | Time between syncs, e.g. `30m`, `1h`. |
| `NOTIFY_URLS` | no | | Comma-separated [shoutrrr](https://containrrr.dev/shoutrrr/) URLs, notified when books can't be matched. |
| `MATCH_CONFIDENCE` | no | `strict` | Fuzzy title/author match threshold: `strict` (0.95), `medium` (0.75), `loose` (0.55), or a number. |
| `WEB_UI_PORT` | no | `8080` | Port the web UI listens on inside the container. |
| `STATE_DB_PATH` | no | `/data/state.db` | Sync state database. |
| `CREDENTIALS_PATH` | no | `/data/credentials.json` | Saved Audible login. |

## Commands

The image's entrypoint is `sync-tool`; the default command is `serve`.

| Command | What it does |
|---|---|
| `docker compose run --rm sync-tool auth` | Log in to Audible (once, or again if the login expires). |
| `docker compose run --rm sync-tool sync` | Run one sync now and exit. |
| `docker compose up -d` | Run the web UI and sync on the `SYNC_INTERVAL` schedule. |

## Updating

```bash
docker compose pull && docker compose up -d
```

Images are tagged `latest` (the `master` branch), `sha-<commit>`, and a
version (`1.2.3`, `1.2`) for each release. Pin a version tag for
predictable updates.

## Troubleshooting

- **`config error`** on start: `HARDCOVER_API_TOKEN` is missing from
  `.env`.
- **`load audible credentials (run 'sync-tool auth' first)`**: the Audible
  login step hasn't been done for this data volume.
- **Audible errors after it worked before**: the saved login expired. Run
  the `auth` command again.
- **Lots of `unmatched`**: Hardcover search was down or the books aren't in
  its catalog. Unmatched books are retried every sync; match the rest by
  hand in the web UI.
- **`status 429` / `408` in the logs**: Hardcover rate limits or timeouts.
  Lookups are retried; failed writes are picked up on the next sync.
- **Start over**: stop the container and delete `state.db` from the data
  volume. Your Hardcover library is not changed, and the next sync
  re-checks everything.

## License

MIT — see [LICENSE](LICENSE).

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
  - Re-listens add a new read. When a book is Read on Hardcover and you
    start it again on Audible (back at 10% or less), or start the audiobook
    of a book you read elsewhere, the sync adds a new read to the entry,
    keeps its progress moving forward, and finishes it when Audible does.
    The entry stays Read. Progress synced to Hardcover from other apps
    (Kobo, CWA) never starts a re-listen.
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
