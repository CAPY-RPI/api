# CAPY API

The backend API for the CAPY club management system. Built with Go (Chi), PostgreSQL, and SQLC.

## Features
- **Authentication**: JWT-based auth and OAuth2 (Google/Microsoft) support.
- **Role-Based Access**: Granular permissions for Student, Alumni, Faculty, and External roles.
- **Organization Management**: Create and join organizations (clubs).
- **Event System**: Event scheduling, registration, and attendance tracking.
- **Bot Integration**: API tokens for bot automation.

## Tech Stack
- **Language**: Go 1.25+
- **Router**: [Chi](https://github.com/go-chi/chi)
- **Database**: PostgreSQL
- **ORM-ish**: [sqlc](https://sqlc.dev/) (Type-safe SQL generation)
- **Migration**: [golang-migrate](https://github.com/golang-migrate/migrate)
- **Testing**:
    - [Testify](https://github.com/stretchr/testify) (Assertions)
    - [Testcontainers](https://github.com/testcontainers/testcontainers-go) (Integration tests)
    - [Mockery](https://github.com/vektra/mockery) (Mock generation)
- **Documentation**: Swagger/OpenAPI (via `swag`)

## Prerequisites
- Go 1.25+
- Docker & Docker Compose (for local DB)
- Make

## Getting Started

### 1. Clone & Config
```bash
git clone https://github.com/CAPY-RPI/api.git
cd capy-api
cp .env.example .env
# Edit .env with your local credentials if needed
```

### 2. Start Infrastructure
Start the PostgreSQL database:
```bash
make docker
```

### 3. Run Migrations & Generate Code
```bash
# Install tools if needed (see Step 5 in agents.md or CI workflow)
make generate
```

### 4. Run Server
```bash
make run
# API will be available at http://localhost:8080
# Health check: http://localhost:8080/health
```

## Testing

### Unit Tests
Fast tests running in isolation with mocks.
```bash
make test
```

### Integration Tests
Full-stack tests using ephemeral Docker containers (Postgres). Requires Docker to be running.
```bash
make test-integration
```

### Run All Tests
```bash
make test-all
```

## Project Structure
```
.
├── cmd/server/         # Main entry point
├── internal/
│   ├── config/         # Configuration loading
│   ├── database/       # sqlc generated code & queries
│   ├── dto/            # Data Transfer Objects (Request/Response)
│   ├── handler/        # HTTP Handlers
│   ├── middleware/     # Auth, CORS, Logger middleware
│   ├── router/         # Route definitions
│   └── testutils/      # Testing helpers
├── migrations/         # SQL migration files
├── tests/integration/  # End-to-end integration tests
└── schema.sql          # Current database schema
```

## CI/CD
This project uses GitHub Actions for continuous integration.
- **Workflow**: `.github/workflows/ci.yml`
- **Checks**: Linting (`golangci-lint`), Unit Tests, Integration Tests.
