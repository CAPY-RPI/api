package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/capyrpi/api/internal/config"
	"github.com/capyrpi/api/internal/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/joho/godotenv"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	envFile := flag.String("env", ".env", "Path to .env file")
	name := flag.String("name", "", "Bot token name (required)")
	createdBy := flag.String("created-by", "", "Creator user ID (UUID, required)")
	hours := flag.Int("hours", 0, "Token validity in hours (0 means no expiry)")
	flag.Parse()

	if *name == "" {
		log.Fatal("name is required")
	}

	creatorID, err := uuid.Parse(*createdBy)
	if err != nil {
		log.Fatalf("invalid created-by UUID: %v", err)
	}

	if *hours < 0 {
		log.Fatal("hours must be >= 0")
	}

	if err := godotenv.Load(*envFile); err != nil {
		log.Printf("Warning: Error loading .env file: %v", err)
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	ctx := context.Background()
	pool, err := database.NewPool(ctx, cfg.Database.URL)
	if err != nil {
		log.Fatalf("Unable to connect to database: %v", err)
	}
	defer pool.Close()

	rawToken, err := generateSecureToken(32)
	if err != nil {
		log.Fatalf("Failed to generate token: %v", err)
	}

	hashedToken, err := bcrypt.GenerateFromPassword([]byte(rawToken), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("Failed to hash token: %v", err)
	}

	token, err := database.New(pool).CreateBotToken(ctx, database.CreateBotTokenParams{
		TokenHash: string(hashedToken),
		Name:      *name,
		CreatedBy: creatorID,
		ExpiresAt: expiryTimestamp(*hours),
	})
	if err != nil {
		log.Fatalf("Failed to create bot token: %v", err)
	}

	fmt.Println("\nBot token created successfully.")
	fmt.Println("---------------------------------------------------")
	fmt.Printf("Token ID:   %s\n", token.TokenID)
	fmt.Printf("Name:       %s\n", token.Name)
	fmt.Printf("Created By: %s\n", token.CreatedBy)
	if token.ExpiresAt.Valid {
		fmt.Printf("Expires At: %s\n", token.ExpiresAt.Time.Format(time.RFC3339))
	} else {
		fmt.Println("Expires At: never")
	}
	fmt.Println("---------------------------------------------------")
	fmt.Println("\nUsage with curl:")
	fmt.Printf("curl -H \"X-Bot-Token: %s\" http://localhost:8080/api/v1/bot/me\n", formatBotToken(token.TokenID, rawToken))
	fmt.Println("\nToken:")
	fmt.Println(formatBotToken(token.TokenID, rawToken))
}

func generateSecureToken(byteLen int) (string, error) {
	buf := make([]byte, byteLen)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func expiryTimestamp(hours int) pgtype.Timestamp {
	if hours == 0 {
		return pgtype.Timestamp{Valid: false}
	}

	return pgtype.Timestamp{
		Time:  time.Now().Add(time.Duration(hours) * time.Hour),
		Valid: true,
	}
}

func formatBotToken(tokenID uuid.UUID, secret string) string {
	return tokenID.String() + "." + secret
}
