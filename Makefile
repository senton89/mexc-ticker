BIN      := bin/collector
PKG      := ./cmd/collector
GOFLAGS  := -trimpath

.PHONY: all lint test build run db-up db-down tidy clean

all: lint test build

lint:
	golangci-lint run ./...

test:
	go test -race -count=1 -timeout 120s ./...

build:
	CGO_ENABLED=0 go build $(GOFLAGS) -ldflags="-s -w" -o $(BIN) $(PKG)

run:
	go run $(PKG)

db-up:
	docker compose up -d postgres

db-down:
	docker compose down -v

tidy:
	go mod tidy

clean:
	rm -rf bin/