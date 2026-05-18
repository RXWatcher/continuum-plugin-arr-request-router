BINARY := continuum-plugin-arr-request-router
GO ?= go
PNPM ?= pnpm

.PHONY: build test test-go test-web clean

build: web-build
	$(GO) build -o $(BINARY) ./cmd/continuum-plugin-arr-request-router

web-build:
	cd web && CI=true $(PNPM) install --frozen-lockfile && $(PNPM) run build

test: test-go test-web

test-go:
	$(GO) test ./...

test-web:
	cd web && $(PNPM) run test --run

clean:
	rm -f $(BINARY)
	rm -rf web/dist
