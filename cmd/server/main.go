package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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

// @host      api.capyrpi.org
// @BasePath  /v1

// @securityDefinitions.apikey CookieAuth
// @in cookie
// @name capy_auth

// @securityDefinitions.apikey BotToken
// @in header
// @name X-Bot-Token
func main() {
	// Setup structured logging
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	if len(os.Args) > 1 {
		os.Exit(runCommand(os.Args[1:]))
	}

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	slog.Info("starting server", "env", cfg.Env)

	ctx := context.Background()

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

func runCommand(args []string) int {
	if len(args) == 0 {
		return 0
	}

	switch args[0] {
	case "migrate":
		if len(args) != 2 || args[1] != "up" {
			slog.Error("invalid migrate command", "usage", "capy-server migrate up")
			return 2
		}

		cfg, err := config.Load()
		if err != nil {
			slog.Error("failed to load config", "error", err)
			return 1
		}

		slog.Info("running migrations", "path", cfg.Database.MigrationsPath)
		if err := database.RunMigrations(context.Background(), cfg.Database.URL, cfg.Database.MigrationsPath); err != nil {
			slog.Error("migration command failed", "error", err)
			return 1
		}

		slog.Info("migrations complete")
		return 0
	default:
		slog.Error("unknown command", "command", args[0], "usage", "capy-server [migrate up]")
		return 2
	}
}
