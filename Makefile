.PHONY: generate build run test test-integration test-all lint swagger docker docker-down ci benchmark

# Code generation
generate:	
	sqlc generate
	swag init -g cmd/server/main.go --output docs/swagger

docs: generate

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
	go test -tags=integration ./tests/integration/... -coverprofile=coverage.out -coverpkg=./...
	go tool cover -html=coverage.out

test-all:
	go test -v -race -tags=integration ./...

benchmark:
	@mkdir -p benchmarks/results
	@timestamp=$$(date +%Y-%m-%d-%H%M%S); \
	log_file="benchmarks/results/benchmark-$$timestamp.txt"; \
	echo "Running benchmarks and saving to $$log_file..."; \
	go test -bench=. -benchmem -run=^$$ ./tests/benchmarks/... | tee $$log_file

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
