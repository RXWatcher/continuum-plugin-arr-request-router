BINARY := continuum-plugin-arr-request-router
GO ?= go
PNPM ?= pnpm

.PHONY: build test clean

build: web-build
	$(GO) build -o $(BINARY) ./cmd/continuum-plugin-arr-request-router

web-build:
	cd web && CI=true $(PNPM) install --frozen-lockfile && $(PNPM) run build

test:
	$(GO) test ./...

clean:
	rm -f $(BINARY)
	rm -rf web/dist
