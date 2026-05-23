.PHONY: build build-web dev dev-api dev-web dev-prereqs install clean clean-dev

# --- production ---
build: build-web
	go build -o bin/argus ./cmd/argus

build-web:
	cd web && npm install && npm run build

install: build
	install -d $(HOME)/.local/bin
	install bin/argus $(HOME)/.local/bin/argus

# --- local development ---
# Ensure web deps + a placeholder embed dir exist so `go build` compiles.
# (internal/web/dist is required by //go:embed; Vite serves the real SPA in dev.)
dev-prereqs:
	@test -d web/node_modules || (cd web && npm install)
	@mkdir -p internal/web/dist
	@test -f internal/web/dist/index.html || \
		printf '%s\n' '<!doctype html><title>argus dev</title>Served by Vite in dev.' \
		> internal/web/dist/index.html

# Full stack: backend hot-reload (air) + frontend HMR (vite), isolated under ./.dev
dev: dev-prereqs
	ARGUS_HOME=$(CURDIR)/.dev go tool hivemind

# Backend hot-reload only
dev-api: dev-prereqs
	ARGUS_HOME=$(CURDIR)/.dev ARGUS_SERVER_PORT=3100 go tool air

# Frontend dev server only
dev-web:
	cd web && ARGUS_SERVER_PORT=3100 ARGUS_WEB_PORT=5273 npm run dev

clean:
	rm -rf bin/ internal/web/dist/ tmp/

# Wipe local dev state: db, sessions, worktrees, discovery file, and any
# personal .dev/config.toml. Separate from `clean` so a routine build cleanup
# never destroys hand-authored dev config or secrets.
clean-dev:
	rm -rf .dev/
