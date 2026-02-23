.PHONY: build build-web dev dev-web clean

build: build-web
	go build -o bin/argus ./cmd/argus

build-web:
	cd web && npm run build

dev: build-web
	go run ./cmd/argus --port 3000

dev-web:
	cd web && npm run dev

clean:
	rm -rf bin/ internal/web/dist/
