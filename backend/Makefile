.PHONY: build build-gateway build-control build-worker build-billing build-bootstrap generate test test-unit test-integration test-e2e

VERSION ?= $(shell tr -d '\r\n' < ./cmd/gateway/VERSION)
LDFLAGS ?= -s -w -X main.Version=$(VERSION)

build: build-gateway build-control build-worker build-billing

build-gateway:
	CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -trimpath -o bin/gateway ./cmd/gateway

build-control:
	CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -trimpath -o bin/control ./cmd/control

build-worker:
	CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -trimpath -o bin/worker ./cmd/worker

build-billing:
	CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -trimpath -o bin/billing ./cmd/billing

build-bootstrap:
	CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -trimpath -o bin/bootstrap ./cmd/bootstrap

generate:
	go generate ./ent
	go generate ./internal/app/gateway
	go generate ./internal/app/control
	go generate ./internal/app/worker
	go generate ./internal/app/billing

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
