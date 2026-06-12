GO      ?= go
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS  = -ldflags "-X main.version=$(VERSION)"

.PHONY: build test test-it test-all test-coverage-cases zooid fmt vet run

build:
	CGO_ENABLED=1 $(GO) build $(LDFLAGS) -o bin/communityd ./cmd/communityd

run: build
	./bin/communityd -data ./data -addr :8080

test:
	CGO_ENABLED=1 $(GO) test ./...

test-it:
	CGO_ENABLED=1 $(GO) test -tags integration ./tests/...

test-all: test test-it test-coverage-cases

test-coverage-cases:
	./scripts/check-case-coverage.sh

zooid:
	./scripts/build-zooid.sh

fmt:
	$(GO) fmt ./...

vet:
	CGO_ENABLED=1 $(GO) vet ./...
