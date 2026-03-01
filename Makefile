BIN     := bin/dbx-dash
CMD     := ./cmd/dbx-dash
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

.PHONY: build run install test lint snapshot clean

build:
	go build -ldflags "-X main.version=$(VERSION)" -o $(BIN) $(CMD)

run: build
	$(BIN) run

install:
	go install -ldflags "-X main.version=$(VERSION)" $(CMD)

test:
	go test ./...

lint:
	golangci-lint run

snapshot:
	goreleaser release --clean --snapshot

clean:
	rm -rf bin/ dist/
