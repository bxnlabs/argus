# argus
Monorepo for Argus platform

## Local development

Requires Go 1.25+ and Node (which ships corepack). One-time setup activates the
repo-pinned pnpm from `web/package.json`:

    corepack enable

Then one command starts the full stack with hot-reload:

    make dev

- **Backend** (combined SPA + node API) runs on http://localhost:3100 with Go
  hot-reload via `air` — editing a `.go` file rebuilds and restarts it.
- **Frontend** (Vite) runs on http://localhost:5273 with HMR and proxies
  API/WS calls to the backend.

Open http://localhost:5273 in your browser. Press Ctrl-C to stop both
processes.

Run a single side alone with `make dev-api` (backend) or `make dev-web`
(frontend).

### Isolation from production

The dev stack runs fully isolated from a production Argus instance on the same
machine: it uses different ports and writes all state — database, sessions, git
worktrees, discovery file — under the gitignored `./.dev/` directory via the
`ARGUS_HOME` environment variable. Your production `~/.argus` is never touched.

To run the CLI against the dev node, point it at the same state dir:

    ARGUS_HOME=$PWD/.dev go run ./cmd/argus session ls

Optional personal overrides (notifications, branch prefix) can be placed in
`.dev/config.toml` — see `argus.dev.toml.example`.

The dev tooling (`air`, `hivemind`) is pinned via Go tool directives in
`go.mod`; the first `make dev` may take a moment to build them.
