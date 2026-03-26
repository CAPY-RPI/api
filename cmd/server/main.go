package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	swaggerdocs "github.com/capyrpi/api/docs/swagger"
	"github.com/capyrpi/api/internal/config"
	"github.com/capyrpi/api/internal/database"
	"github.com/capyrpi/api/internal/handler"
	"github.com/capyrpi/api/internal/router"
)

// @title           Capy API
// @version         1.0
// @description     API for Capy RPI Club Assistant
// @termsOfService  http://swagger.io/terms/

// @contact.name   API Support
// @contact.url    http://www.swagger.io/support
// @contact.email  support@swagger.io

// @license.name  Apache 2.0
// @license.url   http://www.apache.org/licenses/LICENSE-2.0.html

// @host      capyrpi.org
// @BasePath  /api/v1

// @securityDefinitions.apikey CookieAuth
// @in cookie
// @name capy_auth

// @securityDefinitions.apikey BotToken
// @in header
// @name X-Bot-Token
func main() {
	// Setup structured logging
	level := slog.LevelInfo
	if os.Getenv("ENV") == "development" || os.Getenv("ENV") == "staging" || os.Getenv("ENV") == "" {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})))

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	slog.Info("starting server", "env", cfg.Env)

	configureSwagger(cfg)

	ctx := context.Background()

	if err := database.RunMigrations(ctx, cfg.Database.URL, cfg.Database.MigrationsPath); err != nil {
		slog.Error("failed to run migrations", "error", err, "path", cfg.Database.MigrationsPath)
		os.Exit(1)
	}

	slog.Info("migrations applied", "path", cfg.Database.MigrationsPath)

	// Connect to database
	pool, err := database.NewPool(ctx, cfg.Database.URL)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	slog.Info("connected to database")

	// Create queries and handler
	queries := database.New(pool)
	h := handler.New(queries, cfg)

	// Setup router
	r := router.New(h, queries, cfg.JWT.Secret, cfg.Server.AllowedOrigins)

	// Create server
	addr := cfg.Server.Host + ":" + cfg.Server.Port
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		slog.Info("server listening", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("server forced to shutdown", "error", err)
	}

	slog.Info("server stopped")
}

func configureSwagger(cfg *config.Config) {
	if cfg.Env != "development" {
		return
	}

	swaggerdocs.SwaggerInfo.Host = "localhost:" + cfg.Server.Port
	swaggerdocs.SwaggerInfo.Schemes = []string{"http"}
}
