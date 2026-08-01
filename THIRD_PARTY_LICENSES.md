# Third-Party Licenses

This project vendors and depends on the following third-party software.
All licenses are permissive or weak-copyleft (MIT, BSD-3-Clause,
MPL-2.0, or OFL-1.1). Full license text for each is available at the
linked source.

## Vendored source

- **[mstrhakr/go-audible](https://github.com/mstrhakr/go-audible)** (MIT)
  — vendored with modification at `third_party/go-audible/`, which
  retains its own `LICENSE` file per the MIT license's terms. See
  `third_party/go-audible/LICENSE` for the full text and copyright
  notice.

## Go dependencies (compiled into the binary)

| Package | License |
|---|---|
| github.com/containrrr/shoutrrr | MIT |
| github.com/robfig/cron/v3 | MIT |
| github.com/adrg/strutil | MIT |
| github.com/dustin/go-humanize | MIT |
| github.com/fatih/color | MIT |
| github.com/mattn/go-colorable | MIT |
| github.com/mattn/go-isatty | MIT |
| github.com/ncruces/go-strftime | MIT |
| github.com/hashicorp/go-retryablehttp | MPL-2.0 |
| github.com/hashicorp/go-cleanhttp | MPL-2.0 |
| github.com/google/uuid | BSD-3-Clause |
| github.com/remyoudompheng/bigfft | BSD-3-Clause |
| golang.org/x/crypto | BSD-3-Clause |
| golang.org/x/sys | BSD-3-Clause |
| modernc.org/sqlite | BSD-3-Clause |
| modernc.org/libc | BSD-3-Clause |
| modernc.org/mathutil | BSD-3-Clause |
| modernc.org/memory | BSD-3-Clause |

## Frontend dependencies (bundled into the embedded `dist/`)

| Package | License |
|---|---|
| react | MIT |
| react-dom | MIT |
| tailwindcss | MIT |
| @tailwindcss/vite | MIT |
| @fontsource/fraunces | OFL-1.1 |
| @fontsource/ibm-plex-mono | OFL-1.1 |
| @fontsource/ibm-plex-sans | OFL-1.1 |

Build-time-only frontend tooling (`@testing-library/*`, `@types/*`,
`@vitejs/plugin-react`, `jsdom`, `oxlint`, `typescript`, `vite`,
`vitest`) does not ship in the built output and is not listed here.

## MIT/BSD-3-Clause notice

MIT and BSD-3-Clause licenses require this notice: the above software is
provided "as is", without warranty of any kind. Each package's full
license text and copyright holder can be found at its linked repository.

## MPL-2.0 notice

The HashiCorp packages (go-retryablehttp, go-cleanhttp) are licensed
under the Mozilla Public License 2.0, a weak file-level copyleft
license. Using them as an imported Go module does not propagate MPL
obligations to this project's own source files. Each package's full
license text is available at its linked repository.

## OFL-1.1 notice

The bundled fonts (Fraunces, IBM Plex Mono, IBM Plex Sans) are licensed
under the SIL Open Font License 1.1, which permits bundling, use, and
redistribution as part of a larger software product without requiring
attribution in the product's UI — this file exists as a courtesy record,
not a license requirement.