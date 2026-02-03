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

## API Documentation
Interactive API documentation is available via Swagger UI.

- **Web UI**: [http://localhost:8080/swagger/index.html](http://localhost:8080/swagger/index.html)
- **Raw Spec**: `docs/swagger/swagger.json`
- **Regenerate Docs**:
    ```bash
    make docs
    ```

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

## Development Scripts
Helper scripts are located in the `scripts/` directory.

### Create Development User
Seeds a user into the DB and generates a valid JWT for testing.
```bash
go run scripts/create_dev_user/main.go
```

### Generate Token
Manually generates a JWT for an existing user (by email).
```bash
go run scripts/generate_token/main.go
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

## Deployment / Docker Usage

This project automatically builds and publishes a Docker image to GitHub Container Registry (GHCR) on every push to `main`.

### Authentication (Private Repo)
If the repository or package is private, you must authenticate before pulling:

1.  **Generate a Token**: Go to [GitHub Developer Settings](https://github.com/settings/tokens) and create a **Classic PAT** with `read:packages` scope.
    *   *Note: Fine-grained tokens do not yet fully support GitHub Container Registry.*
2.  **Login**:
    ```bash
    export CR_PAT=YOUR_TOKEN
    echo $CR_PAT | docker login ghcr.io -u YOUR_USERNAME --password-stdin
    ```

    **Windows (PowerShell):**
    ```powershell
    $env:CR_PAT = "YOUR_TOKEN"
    echo $env:CR_PAT | docker login ghcr.io -u YOUR_USERNAME --password-stdin
    ```

    > **Note for Organizations**: PATs are attached to your *user account*, not the organization. If your organization uses SAML SSO, you **must authorize the token** for the organization by clicking "Configure SSO" next to the token in your GitHub settings.

### Pulling the Image
```bash
docker pull ghcr.io/capy-rpi/api:main
```

### Running with Docker
You can run the API using Docker without installing Go on your machine:

```bash
docker run -d \
  --name capy-api \
  -p 8080:8080 \
  -e SERVER_HOST=0.0.0.0 \
  -e SERVER_PORT=8080 \
  -e DATABASE_URL="postgres://user:password@host.docker.internal:5432/capy_db?sslmode=disable" \
  -e JWT_SECRET="your-secret-key" \
  -e ENV=production \
  ghcr.io/capy-rpi/api:main
```

**Windows (PowerShell):**
```powershell
docker run -d `
  --name capy-api `
  -p 8080:8080 `
  -e SERVER_HOST=0.0.0.0 `
  -e SERVER_PORT=8080 `
  -e DATABASE_URL="postgres://user:password@host.docker.internal:5432/capy_db?sslmode=disable" `
  -e JWT_SECRET="your-secret-key" `
  -e ENV=production `
  ghcr.io/capy-rpi/api:main
```

> **Note**: `host.docker.internal` allows the container to access your host machine's localhost (e.g., if running Postgres locally). On Linux, you may need `--add-host=host.docker.internal:host-gateway`.

### Running with Docker Compose (Full Stack)
To run the full stack (API + Postgres + Cloudflare Tunnel) on a separate machine, create a `docker-compose.yml` file.

> **Note**: You will need a Cloudflare Tunnel Token. Add `TUNNEL_TOKEN=your_token` to your environment variables or `.env` file.

```yaml
services:
  db:
    image: postgres:16-alpine
    env_file:
      - .env
    environment:
      POSTGRES_USER: ${POSTGRES_USER}
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
      POSTGRES_DB: ${POSTGRES_DB}
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: [ "CMD-SHELL", "pg_isready -U ${POSTGRES_USER} -d ${POSTGRES_DB}" ]
      interval: 5s
      timeout: 5s
      retries: 5

  api:
    image: ghcr.io/capy-rpi/api:main
    ports:
      - "8080:8080"
    env_file:
      - .env
    depends_on:
      db:
        condition: service_healthy

  tunnel:
    image: cloudflare/cloudflared:latest
    restart: unless-stopped
    command: tunnel run
    env_file:
      - .env
    depends_on:
      - api

volumes:
  pgdata:
```
