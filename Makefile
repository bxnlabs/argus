.PHONY: build build-web dev dev-web install clean

build: build-web
	go build -o bin/argus ./cmd/argus

build-web:
	cd web && npm run build

dev: build-web
	go run ./cmd/argus --port 3000

dev-web:
	cd web && npm run dev

install: build
	install -d $(HOME)/.local/bin
	install bin/argus $(HOME)/.local/bin/argus

clean:
	rm -rf bin/ internal/web/dist/
