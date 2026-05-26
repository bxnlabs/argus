.PHONY: build build-web install clean dev-prereqs dev dev-api dev-web dev-clean

# --- production ---
build: build-web
	go build -o bin/argus ./cmd/argus

build-web:
	cd web && npm install && npm run build

install: build
	install -d $(HOME)/.local/bin
	install bin/argus $(HOME)/.local/bin/argus

clean:
	rm -rf bin/ internal/web/dist/

# --- local development ---
# Dev env, declared once per target. `make dev` runs hivemind, which inherits
# this env and passes it to the Procfile's api/web processes, so the ports live
# here only -- not duplicated in the Procfile or the recipes below.
dev dev-api dev-web: export ARGUS_SERVER_PORT = 3100
dev dev-web:         export ARGUS_WEB_PORT = 5273
dev dev-api:         export ARGUS_HOME = $(CURDIR)/.dev

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
	go tool hivemind

# Backend hot-reload only
dev-api: dev-prereqs
	go tool air

# Frontend dev server only
dev-web:
	cd web && npm run dev

# Wipe local dev state: db, sessions, worktrees, discovery file, and any
# personal .dev/config.toml. Separate from `clean` so a routine build cleanup
# never destroys hand-authored dev config or secrets.
dev-clean:
	rm -rf .dev/
