BINARY := continuum-plugin-arr-request-router
GO ?= go

.PHONY: build test clean

build:
	$(GO) build -o $(BINARY) ./cmd/continuum-plugin-arr-request-router

test:
	$(GO) test ./...

clean:
	rm -f $(BINARY)
