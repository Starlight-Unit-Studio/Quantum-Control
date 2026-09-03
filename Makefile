SHELL := /bin/sh

.PHONY: fmt fmt-check legal vet test build check

fmt:
	gofmt -w ./cmd ./internal

fmt-check:
	@test -z "$$(gofmt -l ./cmd ./internal)" || { echo "Go files require gofmt:"; gofmt -l ./cmd ./internal; exit 1; }

legal:
	sh scripts/verify-legal.sh

vet:
	go vet ./...

test:
	go test -race ./...

build:
	mkdir -p bin
	go build -trimpath -o bin/quantum-control ./cmd/quantum-control
	go build -trimpath -o bin/qcored ./cmd/qcored

check:
	sh scripts/verify-legal.sh
	test "$$(cat VERSION)" = "$$(go run ./cmd/quantum-control version)"
	test "$$(cat VERSION)" = "$$(go run ./cmd/qcored version)"
	@test -z "$$(gofmt -l ./cmd ./internal)" || { echo "Go files require gofmt:"; gofmt -l ./cmd ./internal; exit 1; }
	go vet ./...
	go test -race ./...
	@tmpdir="$$(mktemp -d)"; trap 'rm -rf "$$tmpdir"' EXIT HUP INT TERM; \
		go build -trimpath -o "$$tmpdir/quantum-control" ./cmd/quantum-control; \
		go build -trimpath -o "$$tmpdir/qcored" ./cmd/qcored
