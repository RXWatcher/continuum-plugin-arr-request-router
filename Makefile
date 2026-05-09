BINARY := continuum-plugin-arrouter
GO ?= go

.PHONY: build test clean

build:
	$(GO) build -o $(BINARY) ./cmd/continuum-plugin-arrouter

test:
	$(GO) test ./...

clean:
	rm -f $(BINARY)
