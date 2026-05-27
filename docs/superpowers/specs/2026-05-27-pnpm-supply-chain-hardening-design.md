# Migrate argus/web from npm to pnpm + supply-chain hardening (BXN-90)

## Overview

`argus/web` currently installs with npm. npm cannot enforce several
supply-chain defenses that would most reliably have blocked recent worms (e.g.
Shai-Hulud), which depend on installs landing within hours of a malicious
publish and on postinstall lifecycle scripts running unchecked.

Migrate the single `web/` package from npm to **pnpm 10.33.0** and apply a
hardening profile: a publish-age delay, a verify-before-run check, a strict
build-script allowlist, and carried-over dependency overrides. The change is
confined to `web/` plus the three live places that invoke the package manager
(`Makefile`, `Procfile`, `README.md`). No CI or container config exists to
update.

## Goals

- Replace `web/package-lock.json` with a committed `web/pnpm-lock.yaml`.
- Add `web/pnpm-workspace.yaml` holding the hardening settings, each commented.
- Pin the pnpm version via a `packageManager` field, activated through corepack.
- Block all dependency build/lifecycle scripts except an explicit, justified
  allowlist; never enable a run-all escape hatch.
- Make the production build reproducible (frozen lockfile) and keep dev flows
  working with the new verify-before-run check.
- Remove every `npm` invocation from live scripts and docs.

## Non-Goals

- **No CI changes** — `.github/workflows/` does not exist (confirmed).
- **No container changes** — there is no Dockerfile.
- **No dependency version bumps** beyond what pnpm's resolver picks under the
  publish-age delay, plus targeted overrides for vulnerable transitives that
  actually resolve into the tree.
- **No edits to historical plan docs.** The four `npm run build` references in
  `docs/superpowers/plans/*.md` are dated snapshots and stay as-is.
- **Not converting `web/` into a multi-package workspace.** The
  `pnpm-workspace.yaml` exists only to hold root settings.

## Toolchain & Approach

The migration is a prescribed config change, not an architecture choice, so
there is no alternative *structure* to weigh. The decisions that mattered were
settled during brainstorming:

- **Version activation:** corepack. A `packageManager` field pins
  `pnpm@10.33.0`; `corepack enable` makes a fresh clone use exactly that
  version with no manual install. Node 26 and corepack 0.35 are already present.
- **Doc scope:** live files only (`Makefile`, `Procfile`, `README.md`).
- **Makefile semantics:** hardened (frozen-lockfile production build; dev-web
  ensures deps), not a literal 1:1 swap.

Field names below were verified by grepping the cached pnpm **10.33.0** dist —
the version we pin. Notably the Linear issue's `allowBuilds` is **not** a real
key; the correct field is `onlyBuiltDependencies`. Confirmed present in 10.33.0:
`minimumReleaseAge`, `verifyDepsBeforeRun`, `onlyBuiltDependencies`,
`strictDepBuilds`, `dangerouslyAllowAllBuilds`.

## `web/pnpm-workspace.yaml` (new)

In pnpm 10, project settings live in `pnpm-workspace.yaml` rather than `.npmrc`
or `package.json`. The file is settings-only — no `packages:` key, since `web/`
is a single package.

```yaml
# pnpm config + supply-chain hardening for argus/web.
# In pnpm 10, project settings live here (not .npmrc / package.json).

# Wait 5 days (7200 minutes) before installing a freshly published version.
# Primary defense against worms (e.g. Shai-Hulud) that rely on installs
# landing within hours of a malicious publish.
minimumReleaseAge: 7200

# Verify node_modules matches the lockfile before every `pnpm run`.
# Hard error (not warn) on drift or tampering.
verifyDepsBeforeRun: error

# Only these packages may run install/build lifecycle scripts; everything
# else is blocked (pnpm 10 default). Keep minimal and justified — expand
# only when `pnpm install` reports a genuinely-needed build.
onlyBuiltDependencies:
  - esbuild                 # vite's bundler — selects its native binary
  - "@tailwindcss/oxide"    # tailwind v4 native engine
  # (additional entries added only if the resolved tree requires them)

# Error if a non-allowlisted dependency wants to build, rather than
# silently skipping it — forces an explicit decision.
strictDepBuilds: true

# Advisory floors for vulnerable transitives, caret-bounded so a re-resolve
# can't pull an unreviewed major. dompurify carried over from the old
# package.json "overrides"; rollup added empirically (see below).
overrides:
  dompurify: "^3.4.0"
  rollup: "^4.22.4"   # DOM-clobbering XSS fix (GHSA-gcx4-mw62-g8wm)

# NEVER add `dangerouslyAllowAllBuilds: true` — it re-enables the
# run-all-scripts behavior pnpm 10 deliberately disabled.
```

Two items are resolved empirically at the first `pnpm install`, not guessed:

- **`packages:` key.** Default to omitting it (settings-only file). If pnpm
  rejects a `pnpm-workspace.yaml` without `packages`, fall back to
  `packages: ['.']`.
- **`onlyBuiltDependencies` contents.** Driven by pnpm's "ignored build
  scripts" report. Allowlist only legitimate native/bundler builds. **Playwright
  is intentionally excluded** — no e2e script is wired up, so its postinstall
  browser download stays blocked, keeping that surface closed.

## `web/package.json`

- Add `"packageManager": "pnpm@10.33.0+sha512…"`, written via
  `corepack use pnpm@10.33.0` so the integrity hash is real.
- **Remove** the top-level npm-style `"overrides"` block — pnpm ignores it; the
  constraint now lives in `pnpm-workspace.yaml`.
- Scripts (`dev`, `build`, `test`, `preview`) are unchanged; they are
  package-manager-agnostic.

## Lockfile

- Delete `web/package-lock.json`.
- Generate `web/pnpm-lock.yaml` via `pnpm install` (with the hardening settings
  active, so the publish-age-respecting resolution is baked into the lockfile).
- Commit both the removal and the new lockfile.

## Vulnerable transitive overrides

After install, inspect the resolved tree (`pnpm why <pkg>` / `pnpm-lock.yaml`)
for `pbkdf2`, `sha.js`, `serialize-javascript`, and `rollup`. Add an override
under `overrides:` **only if** the package actually resolves in *and* a patched
version exists, each documented inline. `rollup` ships with vite and will
likely be pinned; the rest probably do not appear. No speculative overrides.

## Makefile / Procfile / README

`Makefile` — production build uses a frozen lockfile (the pnpm equivalent of
`npm ci`); `dev-web` gains the `dev-prereqs` dependency so the new
`verifyDepsBeforeRun: error` produces a clean install rather than a confusing
mid-run error:

```make
build-web:
	cd web && pnpm install --frozen-lockfile && pnpm run build

dev-web: dev-prereqs
	cd web && pnpm run dev

dev-prereqs:
	cd web && pnpm install --frozen-lockfile
	@mkdir -p internal/web/dist
	@test -f internal/web/dist/index.html || \
		printf '%s\n' '<!doctype html><title>argus dev</title>Served by Vite in dev.' \
		> internal/web/dist/index.html
```

`Procfile`:

```
web: cd web && pnpm run dev
```

`README.md` — add a one-time `corepack enable` step to the local-development
section and note that pnpm auto-activates at the pinned version. Adjust the
"Requires Go 1.25+ and Node" line to mention pnpm via corepack.

Historical `docs/superpowers/plans/*.md` are left untouched.

## Verification (acceptance gate)

- `pnpm install` produces a clean `web/pnpm-lock.yaml`; the install report shows
  only allowlisted build scripts ran and `dangerouslyAllowAllBuilds` is absent.
- `pnpm run build` succeeds (`tsc -b && vite build`), populating
  `internal/web/dist`; `make build` then embeds it into the Go binary.
- `pnpm run test` (vitest) passes.
- `pnpm run dev` brings vite up on port 5273 — started, confirmed, stopped.
  The full `make dev` / hivemind stack is **not** run: a live production argus
  runs on this machine, and verifying vite directly satisfies the criterion
  without spinning the whole stack.
- `grep` confirms no `npm ` invocations remain in `Makefile`, `Procfile`, or
  `README.md`.

## Risks & Tradeoffs

- **Publish-age delay shifts resolution.** With `minimumReleaseAge: 7200`, the
  initial lockfile may pin versions slightly older than a no-delay resolve for
  any dependency published in the last 5 days. This is the intended behavior.
- **`verifyDepsBeforeRun: error` is strict.** Any standalone `pnpm run` against
  a stale `node_modules` now errors. The Makefile mitigates this for `dev-web`;
  contributors running scripts by hand must `pnpm install` after pulling
  lockfile changes.
- **Corepack dependency.** Activation relies on `corepack enable`. It is bundled
  with Node 26 here; the `packageManager` field still pins the version for any
  tooling that honors it even without corepack.
- **`onlyBuiltDependencies` maintenance.** Adding a future dependency that needs
  a build script will fail loudly (by design) until it is reviewed and
  allowlisted — a deliberate friction, not a bug.

## File Changes

| File | Change |
|------|--------|
| `web/pnpm-workspace.yaml` (new) | Hardening settings: `minimumReleaseAge`, `verifyDepsBeforeRun`, `onlyBuiltDependencies`, `strictDepBuilds`, `overrides`. |
| `web/pnpm-lock.yaml` (new) | Generated lockfile, committed. |
| `web/package-lock.json` (deleted) | Replaced by the pnpm lockfile. |
| `web/package.json` | Add `packageManager`; remove top-level `overrides`. |
| `Makefile` | pnpm invocations; frozen-lockfile build; `dev-web: dev-prereqs`. |
| `Procfile` | `web:` uses `pnpm run dev`. |
| `README.md` | `corepack enable` step; pnpm activation note. |
