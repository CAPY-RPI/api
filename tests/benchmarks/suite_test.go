package benchmarks

import (
	"context"
	"log"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/capyrpi/api/internal/config"
	"github.com/capyrpi/api/internal/database"
	"github.com/capyrpi/api/internal/handler"
	"github.com/capyrpi/api/internal/middleware"
	"github.com/capyrpi/api/internal/router"
	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

var (
	benchDB        *pgxpool.Pool
	benchRouter    chi.Router
	benchServer    *httptest.Server
	benchQueries   *database.Queries
	benchJWTToken  string
	benchUserID    string
	benchOrgID     string
	benchEventID   string
	benchJWTSecret string = "bench-secret"
)

func TestMain(m *testing.M) {
	ctx := context.Background()

	log.Println("Starting benchmark suite setup...")

	_, filename, _, _ := runtime.Caller(0)
	projectRoot := filepath.Join(filepath.Dir(filename), "../..")
	migrationsPath := filepath.Join(projectRoot, "migrations")

	log.Printf("Using migrations from: %s", migrationsPath)

	pgContainer, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("bench_db"),
		postgres.WithUsername("bench"),
		postgres.WithPassword("bench"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second)),
	)
	if err != nil {
		log.Fatalf("failed to start postgres container: %v", err)
	}

	defer func() {
		if r := recover(); r != nil {
			log.Printf("Recovered from panic during benchmark setup/run: %v", r)
		}
		log.Println("Terminating postgres container...")
		if err := pgContainer.Terminate(ctx); err != nil {
			log.Printf("failed to terminate container: %v", err)
		}
	}()

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		log.Fatalf("failed to get connection string: %v", err)
	}

	if err := database.RunMigrations(ctx, connStr, migrationsPath); err != nil {
		log.Fatalf("failed to run migrations: %v", err)
	}

	benchDB, err = database.NewPool(ctx, connStr)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}

	benchQueries = database.New(benchDB)
	setupTestData(ctx)

	cfg := &config.Config{
		Env: "bench",
		JWT: config.JWTConfig{
			Secret: "bench-secret",
		},
		OAuth: config.OAuthConfig{
			Google: config.GoogleOAuthConfig{
				ClientID:     "google-id",
				ClientSecret: "google-secret",
				RedirectURL:  "http://localhost/callback",
			},
			Microsoft: config.MicrosoftOAuthConfig{
				ClientID:     "ms-id",
				ClientSecret: "ms-secret",
				TenantID:     "common",
				RedirectURL:  "http://localhost/callback",
			},
		},
	}

	h := handler.New(benchQueries, cfg)
	benchRouter = router.New(h, benchQueries, cfg.JWT.Secret, []string{"*"})

	benchServer = httptest.NewServer(benchRouter)
	defer benchServer.Close()

	log.Println("Benchmark suite setup complete. Running benchmarks...")
	code := m.Run()

	os.Exit(code)
}

func setupTestData(ctx context.Context) {
	user, err := benchQueries.CreateUser(ctx, database.CreateUserParams{
		FirstName:     "Bench",
		LastName:      "User",
		PersonalEmail: pgtype.Text{String: "bench@example.com", Valid: true},
		Role:          database.NullUserRole{UserRole: database.UserRoleStudent, Valid: true},
	})
	if err != nil {
		log.Fatalf("failed to create user: %v", err)
	}
	benchUserID = user.Uid.String()

	org, err := benchQueries.CreateOrganization(ctx, "Bench Org")
	if err != nil {
		log.Fatalf("failed to create organization: %v", err)
	}
	benchOrgID = org.Oid.String()

	event, err := benchQueries.CreateEvent(ctx, database.CreateEventParams{
		Location:    pgtype.Text{String: "Bench Event", Valid: true},
		EventTime:   pgtype.Timestamp{Time: time.Now().Add(24 * time.Hour), Valid: true},
		Description: pgtype.Text{String: "Benchmark organization", Valid: true},
	})
	if err != nil {
		log.Fatalf("failed to create event: %v", err)
	}
	benchEventID = event.Eid.String()

	err = benchQueries.AddEventHost(ctx, database.AddEventHostParams{
		Eid: event.Eid,
		Oid: org.Oid,
	})
	if err != nil {
		log.Fatalf("failed to add event host: %v", err)
	}

	claims := middleware.UserClaims{
		UserID: benchUserID,
		Email:  "bench@example.com",
		Role:   "student",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	benchJWTToken, err = token.SignedString([]byte(benchJWTSecret))
	if err != nil {
		log.Fatalf("failed to generate token: %v", err)
	}
}
