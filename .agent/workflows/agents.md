---
description: Best practices workflow for building the CAPY Go REST API with type-safe, production-ready code
---

# CAPY API Development Workflow

This workflow ensures type-safe, reliable, and production-ready Go code for the CAPY club management API.

> [!IMPORTANT]
> **NO EMOJIS IN CODE OR COMMITS**
> Do not use emojis in source code comments, commit messages, or pull request descriptions. Keep professional and clean.


## Prerequisites

Ensure you have the required tools installed:
```bash
go version          # Go 1.22+
sqlc version        # sqlc CLI
migrate -version    # golang-migrate CLI
docker --version    # Docker for local dev
```

---

## 1. Database Changes Workflow

When modifying the database schema:

### Step 1.1: Create a New Migration

```bash
migrate create -ext sql -dir migrations -seq <migration_name>
```

### Step 1.2: Write Up Migration

Edit `migrations/XXXXXX_<migration_name>.up.sql` with your DDL statements:
- Use `CREATE TABLE IF NOT EXISTS` for idempotency
- Add proper constraints (NOT NULL, UNIQUE, FOREIGN KEY)
- Include indexes for frequently queried columns

### Step 1.3: Write Down Migration

Edit `migrations/XXXXXX_<migration_name>.down.sql` with rollback statements:
- `DROP TABLE IF EXISTS` in reverse dependency order
- `DROP INDEX IF EXISTS` for any created indexes

### Step 1.4: Apply Migration

```bash
# Ensure DB is running
docker-compose up -d db

# Apply migrations
migrate -path migrations -database "postgres://capy:secret@localhost:5432/capy_db?sslmode=disable" up
```

// turbo-all

---

## 2. sqlc Query Workflow

When adding new database operations:

### Step 2.1: Add Query to queries.sql

Edit `internal/database/queries.sql`. Follow these naming conventions:

```sql
-- CRUD Operations:
-- name: Get<Entity> :one
-- name: List<Entity>s :many
-- name: Create<Entity> :one
-- name: Update<Entity> :one
-- name: Delete<Entity> :exec

-- Example:
-- name: GetUserByEmail :one
SELECT * FROM users WHERE personal_email = $1 OR school_email = $1;

-- name: ListUsersByRole :many
SELECT * FROM users WHERE role = $1 ORDER BY last_name LIMIT $2 OFFSET $3;
```

### Step 2.2: Generate Go Code

```bash
sqlc generate
```

### Step 2.3: Verify Generated Code

Check `internal/database/queries.sql.go` for:
- Correct function signatures
- Proper parameter types (especially UUIDs and custom enums)
- Expected return types

// turbo

---

## 3. Handler Implementation Workflow

When creating a new API endpoint:

### Step 3.1: Define Request/Response Types

Create or update DTOs in the handler file:

```go
type CreateUserRequest struct {
    FirstName     string `json:"first_name" validate:"required,min=1,max=100"`
    LastName      string `json:"last_name" validate:"required,min=1,max=100"`
    PersonalEmail string `json:"personal_email" validate:"omitempty,email"`
    SchoolEmail   string `json:"school_email" validate:"omitempty,email"`
    Phone         string `json:"phone" validate:"omitempty,e164"`
    GradYear      int    `json:"grad_year" validate:"omitempty,gte=2000,lte=2100"`
}

type UserResponse struct {
    UID          uuid.UUID `json:"uid"`
    FirstName    string    `json:"first_name"`
    LastName     string    `json:"last_name"`
    // ... other fields
}
```

### Step 3.2: Implement Handler Function

Follow this pattern:

```go
func (h *Handler) CreateUser(w http.ResponseWriter, r *http.Request) {
    // 1. Parse request body
    var req CreateUserRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        h.respondError(w, http.StatusBadRequest, "Invalid request body")
        return
    }

    // 2. Validate input
    if err := h.validator.Struct(req); err != nil {
        h.respondError(w, http.StatusBadRequest, err.Error())
        return
    }

    // 3. Call database via sqlc
    user, err := h.queries.CreateUser(r.Context(), database.CreateUserParams{
        FirstName:     req.FirstName,
        LastName:      req.LastName,
        PersonalEmail: toNullString(req.PersonalEmail),
        SchoolEmail:   toNullString(req.SchoolEmail),
        Phone:         toNullString(req.Phone),
        GradYear:      toNullInt32(req.GradYear),
    })
    if err != nil {
        h.handleDBError(w, err)
        return
    }

    // 4. Return response
    h.respondJSON(w, http.StatusCreated, toUserResponse(user))
}
```

### Step 3.3: Register Route

Add route to `internal/router/router.go`:

```go
r.Route("/users", func(r chi.Router) {
    r.Use(middleware.Authenticator)
    r.Get("/", h.ListUsers)
    r.Post("/", h.CreateUser)
    r.Route("/{uid}", func(r chi.Router) {
        r.Get("/", h.GetUser)
        r.Put("/", h.UpdateUser)
        r.Delete("/", h.DeleteUser)
    })
})
```

---

## 4. Testing Workflow

### Step 4.1: Unit Tests

Create `*_test.go` files alongside handlers:

```go
func TestCreateUser_Success(t *testing.T) {
    // Setup mock queries
    mockQueries := &database.MockQueries{}
    h := NewHandler(mockQueries)

    // Create request
    body := `{"first_name": "John", "last_name": "Doe"}`
    req := httptest.NewRequest("POST", "/users", strings.NewReader(body))
    rec := httptest.NewRecorder()

    // Execute
    h.CreateUser(rec, req)

    // Assert
    assert.Equal(t, http.StatusCreated, rec.Code)
}
```

### Step 4.2: Integration Tests (Testcontainers)

Use `testcontainers-go` for isolated integration testing without managing external Docker Compose services manually.

```go
//go:build integration

func TestUserAPI_Integration(t *testing.T) {
    // Setup ephemeral Postgres via Testcontainers
    pool := testutils.SetupTestDB(t)
    defer pool.Close()

    // Test CRUD operations logic
    // ...
}
```

### Step 4.3: Run Tests

```bash
# Unit tests only (fast)
go test -v ./...

# All tests including integration (slow, requires Docker)
make test-all
# OR
go test -v -tags=integration ./...
```

// turbo

---

## 5. Security Checklist

Before deploying any endpoint:

- [ ] **Authentication**: Endpoint uses `middleware.Authenticator`
- [ ] **Authorization**: Role check matches API design (faculty, org_admin, etc.)
- [ ] **Input Validation**: All user input validated with struct tags
- [ ] **SQL Injection**: Using sqlc (parameterized queries) — automatic
- [ ] **Error Handling**: No sensitive info leaked in error messages
- [ ] **Logging**: Sensitive data (passwords, tokens) never logged
- [ ] **Rate Limiting**: Consider adding for public endpoints

---

## 6. Code Quality Standards

### Type Safety Rules

1. **Always use `uuid.UUID`** — never raw strings for IDs
2. **Use sqlc-generated types** — don't create duplicate structs
3. **Handle nullable fields** — use `pgtype.Text`, `pgtype.Int4`, etc.
4. **Validate at boundaries** — check all input in handlers

### Error Handling Pattern

```go
// Define domain errors
var (
    ErrUserNotFound = errors.New("user not found")
    ErrDuplicateEmail = errors.New("email already exists")
)

// Wrap database errors
func (h *Handler) handleDBError(w http.ResponseWriter, err error) {
    if errors.Is(err, pgx.ErrNoRows) {
        h.respondError(w, http.StatusNotFound, "Resource not found")
        return
    }
    
    var pgErr *pgconn.PgError
    if errors.As(err, &pgErr) && pgErr.Code == "23505" {
        h.respondError(w, http.StatusConflict, "Duplicate entry")
        return
    }
    
    // Log unexpected errors
    slog.Error("database error", "error", err)
    h.respondError(w, http.StatusInternalServerError, "Internal server error")
}
```

---

## 7. Build & Deploy

### Local Development

```bash
# Start all services
docker-compose up --build

# Watch mode (with air)
air
```

### Production Build

```bash
# Build optimized binary
CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o capy-server ./cmd/server

# Build Docker image
docker build -t capy-api:latest .

# Push to registry
docker tag capy-api:latest registry.example.com/capy-api:v1.0.0
docker push registry.example.com/capy-api:v1.0.0
```

---

## Quick Reference Commands

| Task | Command |
|------|---------|
| Generate sqlc | `sqlc generate` |
| New migration | `migrate create -ext sql -dir migrations -seq name` |
| Apply migrations | `migrate -path migrations -database $DATABASE_URL up` |
| Rollback 1 step | `migrate -path migrations -database $DATABASE_URL down 1` |
| Run tests | `go test -v ./...` |
| Build binary | `go build -o capy-server ./cmd/server` |
| Start dev | `docker-compose up --build` |
| View logs | `docker-compose logs -f api` |
