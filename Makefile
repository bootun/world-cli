.PHONY: build install test fmt vet verify

build:
	go build -trimpath -o bin/world ./cmd/world

install:
	go install ./cmd/world

test:
	go test -race -count=1 ./...

fmt:
	gofmt -w cmd internal

vet:
	go vet ./...

verify:
	@test -z "$$(gofmt -l cmd internal)"
	go vet ./...
	go test -race -count=1 ./...
