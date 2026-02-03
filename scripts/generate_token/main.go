package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/capyrpi/api/internal/config"
	"github.com/capyrpi/api/internal/middleware"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
)

func main() {
	// flags
	envFile := flag.String("env", ".env", "Path to .env file")
	uid := flag.String("uid", "", "User ID (UUID). If empty, generates a random one.")
	email := flag.String("email", "dev@example.com", "User email")
	role := flag.String("role", "student", "User role")
	hours := flag.Int("hours", 24, "Token validity in hours")
	flag.Parse()

	// Load .env if present
	if err := godotenv.Load(*envFile); err != nil {
		log.Printf("Warning: Error loading .env file: %v. Assuming environment variables are set.", err)
	}

	// Load config to get secret
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Determine UID
	var userID string
	if *uid == "" {
		userID = uuid.New().String()
		fmt.Printf("Generated random User ID: %s\n", userID)
	} else {
		userID = *uid
		// Validate UUID
		if _, err := uuid.Parse(userID); err != nil {
			log.Fatalf("Invalid UUID format: %v", err)
		}
	}

	// Create Claims
	claims := middleware.UserClaims{
		UserID: userID,
		Email:  *email,
		Role:   *role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(*hours) * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	// Sign Token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(cfg.JWT.Secret))
	if err != nil {
		log.Fatalf("Failed to sign token: %v", err)
	}

	fmt.Println("\n✅ Token Generated Successfully!")
	fmt.Println("---------------------------------------------------")
	fmt.Printf("User ID: %s\n", userID)
	fmt.Printf("Email:   %s\n", *email)
	fmt.Printf("Role:    %s\n", *role)
	fmt.Println("---------------------------------------------------")
	fmt.Println("\nUsage with curl:")
	fmt.Printf("curl -H \"Authorization: Bearer %s\" http://localhost:8080/v1/auth/me\n", tokenString)
	fmt.Println("\nToken:")
	fmt.Println(tokenString)
}
