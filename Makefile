.PHONY: generate build run test test-integration test-integration-verbose test-all lint swagger docker docker-down ci benchmark \
	migrate-create migrate-up migrate-down migrate-down-all migrate-down-one migrate-version migrate-force \
	migrate-up-docker migrate-down-docker migrate-down-all-docker migrate-down-one-docker migrate-version-docker migrate-force-docker

MIGRATE ?= migrate
MIGRATIONS_DIR ?= migrations
MIGRATE_DATABASE_URL ?=
MIGRATE_DOCKER_IMAGE ?= migrate/migrate
COMPOSE_PROJECT_NAME ?= $(notdir $(CURDIR))
COMPOSE_NETWORK ?= $(COMPOSE_PROJECT_NAME)_default
DOCKER_COMPOSE ?= docker compose

# Code generation
generate:
	sqlc generate
	swag init -g cmd/server/main.go --output docs/swagger

docs: generate

# Database migrations (golang-migrate)
migrate-create:
	@test -n "$(name)" || (echo "Usage: make migrate-create name=add_users_table" && exit 1)
	@mkdir -p $(MIGRATIONS_DIR)
	$(MIGRATE) create -ext sql -dir $(MIGRATIONS_DIR) $(name)

migrate-up:
	@db_url="$${MIGRATE_DATABASE_URL:-$${DATABASE_URL:-$$(grep -E '^DATABASE_URL=' .env 2>/dev/null | head -n1 | cut -d= -f2-)}}"; \
	test -n "$$db_url" || (echo "Set MIGRATE_DATABASE_URL or DATABASE_URL (or add DATABASE_URL to .env)" && exit 1); \
	$(MIGRATE) -path $(MIGRATIONS_DIR) -database "$$db_url" up

migrate-down:
	@$(MAKE) migrate-down-all

migrate-down-all:
	@db_url="$${MIGRATE_DATABASE_URL:-$${DATABASE_URL:-$$(grep -E '^DATABASE_URL=' .env 2>/dev/null | head -n1 | cut -d= -f2-)}}"; \
	test -n "$$db_url" || (echo "Set MIGRATE_DATABASE_URL or DATABASE_URL (or add DATABASE_URL to .env)" && exit 1); \
	$(MIGRATE) -path $(MIGRATIONS_DIR) -database "$$db_url" down -all

migrate-down-one:
	@db_url="$${MIGRATE_DATABASE_URL:-$${DATABASE_URL:-$$(grep -E '^DATABASE_URL=' .env 2>/dev/null | head -n1 | cut -d= -f2-)}}"; \
	test -n "$$db_url" || (echo "Set MIGRATE_DATABASE_URL or DATABASE_URL (or add DATABASE_URL to .env)" && exit 1); \
	$(MIGRATE) -path $(MIGRATIONS_DIR) -database "$$db_url" down 1

migrate-version:
	@db_url="$${MIGRATE_DATABASE_URL:-$${DATABASE_URL:-$$(grep -E '^DATABASE_URL=' .env 2>/dev/null | head -n1 | cut -d= -f2-)}}"; \
	test -n "$$db_url" || (echo "Set MIGRATE_DATABASE_URL or DATABASE_URL (or add DATABASE_URL to .env)" && exit 1); \
	$(MIGRATE) -path $(MIGRATIONS_DIR) -database "$$db_url" version

migrate-force:
	@test -n "$(version)" || (echo "Usage: make migrate-force version=<number>" && exit 1)
	@db_url="$${MIGRATE_DATABASE_URL:-$${DATABASE_URL:-$$(grep -E '^DATABASE_URL=' .env 2>/dev/null | head -n1 | cut -d= -f2-)}}"; \
	test -n "$$db_url" || (echo "Set MIGRATE_DATABASE_URL or DATABASE_URL (or add DATABASE_URL to .env)" && exit 1); \
	$(MIGRATE) -path $(MIGRATIONS_DIR) -database "$$db_url" force $(version)

# Database migrations via Docker (uses Compose network, works with DATABASE_URL=db:5432)
migrate-up-docker:
	@db_url="$${MIGRATE_DATABASE_URL:-$${DATABASE_URL:-$$(grep -E '^DATABASE_URL=' .env 2>/dev/null | head -n1 | cut -d= -f2-)}}"; \
	test -n "$$db_url" || (echo "Set MIGRATE_DATABASE_URL or DATABASE_URL (or add DATABASE_URL to .env)" && exit 1); \
	docker run --rm --network $(COMPOSE_NETWORK) -v "$(CURDIR)/$(MIGRATIONS_DIR):/migrations" $(MIGRATE_DOCKER_IMAGE) -path /migrations -database "$$db_url" up

migrate-down-docker:
	@$(MAKE) migrate-down-all-docker

migrate-down-all-docker:
	@db_url="$${MIGRATE_DATABASE_URL:-$${DATABASE_URL:-$$(grep -E '^DATABASE_URL=' .env 2>/dev/null | head -n1 | cut -d= -f2-)}}"; \
	test -n "$$db_url" || (echo "Set MIGRATE_DATABASE_URL or DATABASE_URL (or add DATABASE_URL to .env)" && exit 1); \
	docker run --rm --network $(COMPOSE_NETWORK) -v "$(CURDIR)/$(MIGRATIONS_DIR):/migrations" $(MIGRATE_DOCKER_IMAGE) -path /migrations -database "$$db_url" down -all

migrate-down-one-docker:
	@db_url="$${MIGRATE_DATABASE_URL:-$${DATABASE_URL:-$$(grep -E '^DATABASE_URL=' .env 2>/dev/null | head -n1 | cut -d= -f2-)}}"; \
	test -n "$$db_url" || (echo "Set MIGRATE_DATABASE_URL or DATABASE_URL (or add DATABASE_URL to .env)" && exit 1); \
	docker run --rm --network $(COMPOSE_NETWORK) -v "$(CURDIR)/$(MIGRATIONS_DIR):/migrations" $(MIGRATE_DOCKER_IMAGE) -path /migrations -database "$$db_url" down 1

migrate-version-docker:
	@db_url="$${MIGRATE_DATABASE_URL:-$${DATABASE_URL:-$$(grep -E '^DATABASE_URL=' .env 2>/dev/null | head -n1 | cut -d= -f2-)}}"; \
	test -n "$$db_url" || (echo "Set MIGRATE_DATABASE_URL or DATABASE_URL (or add DATABASE_URL to .env)" && exit 1); \
	docker run --rm --network $(COMPOSE_NETWORK) -v "$(CURDIR)/$(MIGRATIONS_DIR):/migrations" $(MIGRATE_DOCKER_IMAGE) -path /migrations -database "$$db_url" version

migrate-force-docker:
	@test -n "$(version)" || (echo "Usage: make migrate-force-docker version=<number>" && exit 1)
	@db_url="$${MIGRATE_DATABASE_URL:-$${DATABASE_URL:-$$(grep -E '^DATABASE_URL=' .env 2>/dev/null | head -n1 | cut -d= -f2-)}}"; \
	test -n "$$db_url" || (echo "Set MIGRATE_DATABASE_URL or DATABASE_URL (or add DATABASE_URL to .env)" && exit 1); \
	docker run --rm --network $(COMPOSE_NETWORK) -v "$(CURDIR)/$(MIGRATIONS_DIR):/migrations" $(MIGRATE_DOCKER_IMAGE) -path /migrations -database "$$db_url" force $(version)

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
	go test -tags=integration ./internal/database/... -count=1
	go test -tags=integration ./tests/integration/... -coverprofile=coverage.out -coverpkg=./...
	go tool cover -func=coverage.out

test-integration-verbose:
	go test -v -tags=integration ./internal/database/... -count=1
	go test -v -tags=integration ./tests/integration/... -coverprofile=coverage.out -coverpkg=./...
	go tool cover -func=coverage.out

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
	$(DOCKER_COMPOSE) up --build -d

docker-down:
	$(DOCKER_COMPOSE) down -v

# CI pipeline
ci: lint test-all
