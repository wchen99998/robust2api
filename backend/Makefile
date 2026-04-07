.PHONY: build build-api build-worker build-bootstrap generate test test-unit test-integration test-e2e

VERSION ?= $(shell tr -d '\r\n' < ./cmd/api/VERSION)
LDFLAGS ?= -s -w -X main.Version=$(VERSION)

build: build-api build-worker

build-api:
	CGO_ENABLED=0 go build -tags embed -ldflags="$(LDFLAGS)" -trimpath -o bin/api ./cmd/api

build-worker:
	CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -trimpath -o bin/worker ./cmd/worker

build-bootstrap:
	CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -trimpath -o bin/bootstrap ./cmd/bootstrap

generate:
	go generate ./ent
	go generate ./cmd/api
	go generate ./cmd/worker

test:
	go test ./...
	golangci-lint run ./...

test-unit:
	go test -tags=unit ./...

test-integration:
	go test -tags=integration ./...

test-e2e:
	./scripts/e2e-test.sh

test-e2e-local:
	go test -tags=e2e -v -timeout=300s ./internal/integration/...
