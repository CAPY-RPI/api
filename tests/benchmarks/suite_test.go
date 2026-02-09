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
	"github.com/capyrpi/api/internal/router"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

var (
	benchDB     *pgxpool.Pool
	benchRouter chi.Router
	benchServer *httptest.Server
)

func TestMain(m *testing.M) {
	ctx := context.Background()

	log.Println("Starting benchmark suite setup...")

	_, filename, _, _ := runtime.Caller(0)
	projectRoot := filepath.Join(filepath.Dir(filename), "../..")
	schemaPath := filepath.Join(projectRoot, "schema.sql")

	log.Printf("Using schema from: %s", schemaPath)

	pgContainer, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithInitScripts(schemaPath),
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

	benchDB, err = database.NewPool(ctx, connStr)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}

	queries := database.New(benchDB)

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

	h := handler.New(queries, cfg)
	benchRouter = router.New(h, queries, cfg.JWT.Secret, []string{"*"})

	benchServer = httptest.NewServer(benchRouter)
	defer benchServer.Close()

	log.Println("Benchmark suite setup complete. Running benchmarks...")
	code := m.Run()

	os.Exit(code)
}
