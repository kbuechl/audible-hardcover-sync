# Manual-match UI frontend

React + TypeScript + Tailwind CSS v4, built with Vite.

## Development

```bash
npm install
npm run dev      # Vite dev server on :5173, proxies /api/* to localhost:8080
npm run test     # Vitest
npm run build    # outputs dist/, which the Go binary embeds
```

`dist/` is committed to the repository with a placeholder build so the Go
module always compiles via `go:embed`, even before `npm install`/`npm run
build` has ever been run. Always run `npm run build` after frontend changes
— the Docker image build does this automatically, but local `go run`/`go
test` will serve whatever is currently committed in `dist/` otherwise.
