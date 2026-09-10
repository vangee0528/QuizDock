GO ?= go
NPM ?= npm
BINARY := dist/quizdock

.PHONY: setup dev web build test check clean

setup:
	cd web && $(NPM) ci
	$(GO) mod download

dev:
	@cd web && $(NPM) run dev & \
	  web_pid=$$!; \
	  trap 'kill $$web_pid 2>/dev/null || true' INT TERM EXIT; \
	  $(GO) run ./cmd/quizdock serve --web-dev-url http://127.0.0.1:5173

web:
	cd web && $(NPM) run build

build: web
	mkdir -p dist
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags="-s -w -X main.version=$$(cat VERSION)" -o $(BINARY) ./cmd/quizdock

test:
	$(GO) test ./...
	cd web && $(NPM) test -- --run

check:
	$(GO) fmt ./...
	$(GO) vet ./...
	$(GO) test ./...
	cd web && $(NPM) run typecheck && $(NPM) test -- --run && $(NPM) run build

clean:
	$(GO) clean -testcache
	cd web && $(NPM) run clean
