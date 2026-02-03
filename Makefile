.PHONY: generate build run test test-integration lint swagger docker ci

# Code generation
generate:
	sqlc generate

docs:
	sqlc generate

# Build
build: generate
	go build -o bin/capy-server ./cmd/server

# Development
run:
	go run ./cmd/server

# Testing
test:
	go test -v -race -short ./...

test-integration:
	go test -v -race -tags=integration ./tests/integration/... ./internal/database/...

test-all:
	go test -v -race -tags=integration ./...

# Linting
lint:
	golangci-lint run ./...

# Docker
docker:
	docker-compose up --build -d

docker-down:
	docker-compose down -v

# CI pipeline
ci: lint test-all
