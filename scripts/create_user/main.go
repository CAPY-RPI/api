package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/capyrpi/api/internal/config"
	"github.com/capyrpi/api/internal/database"
	"github.com/capyrpi/api/internal/middleware"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

func main() {
	envFile := flag.String("env", ".env", "Path to .env file")
	email := flag.String("email", "", "User email (required)")
	firstName := flag.String("first-name", "Dev", "User first name")
	lastName := flag.String("last-name", "User", "User last name")
	role := flag.String("role", string(database.UserRoleStudent), "User role")
	hours := flag.Int("hours", 24, "Token validity in hours")
	flag.Parse()

	normalizedEmail := normalizeEmail(*email)
	if normalizedEmail == "" {
		log.Fatal("email is required")
	}

	userRole, err := parseRole(*role)
	if err != nil {
		log.Fatal(err)
	}

	if err := godotenv.Load(*envFile); err != nil {
		log.Printf("Warning: Error loading .env file: %v", err)
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	pool, err := pgxpool.New(context.Background(), cfg.Database.URL)
	if err != nil {
		log.Fatalf("Unable to connect to database: %v", err)
	}
	defer pool.Close()

	queries := database.New(pool)
	ctx := context.Background()
	emailText := pgtype.Text{String: normalizedEmail, Valid: true}

	user, err := queries.GetUserByEmail(ctx, emailText)
	if err != nil {
		if err != pgx.ErrNoRows {
			log.Fatalf("Failed to look up user: %v", err)
		}

		user, err = queries.CreateUser(ctx, database.CreateUserParams{
			FirstName:     *firstName,
			LastName:      *lastName,
			PersonalEmail: emailText,
			Role:          database.NullUserRole{UserRole: userRole, Valid: true},
		})
		if err != nil {
			log.Fatalf("Failed to create user: %v", err)
		}

		printResult("created", user, mustMintToken(user, cfg, *hours))
		return
	}

	user, err = queries.UpdateUser(ctx, database.UpdateUserParams{
		Uid:           user.Uid,
		FirstName:     pgtype.Text{String: *firstName, Valid: true},
		LastName:      pgtype.Text{String: *lastName, Valid: true},
		PersonalEmail: emailText,
		SchoolEmail:   pgtype.Text{Valid: false},
		Phone:         pgtype.Text{Valid: false},
		GradYear:      pgtype.Int4{Valid: false},
		Role:          database.NullUserRole{UserRole: userRole, Valid: true},
	})
	if err != nil {
		log.Fatalf("Failed to update user: %v", err)
	}

	printResult("updated", user, mustMintToken(user, cfg, *hours))
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func parseRole(raw string) (database.UserRole, error) {
	role := database.UserRole(strings.ToLower(strings.TrimSpace(raw)))

	switch role {
	case database.UserRoleStudent, database.UserRoleAlumni, database.UserRoleFaculty, database.UserRoleExternal, database.UserRoleDev:
		return role, nil
	default:
		return "", fmt.Errorf("invalid role %q", raw)
	}
}

func mustMintToken(user database.User, cfg *config.Config, hours int) string {
	claims := middleware.UserClaims{
		UserID: user.Uid.String(),
		Email:  user.PersonalEmail.String,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(hours) * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "capy-api",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(cfg.JWT.Secret))
	if err != nil {
		log.Fatalf("Failed to sign token: %v", err)
	}
	return tokenString
}

func printResult(action string, user database.User, token string) {
	fmt.Printf("\nUser %s successfully.\n", action)
	fmt.Println("---------------------------------------------------")
	fmt.Printf("User Name: %s %s\n", user.FirstName, user.LastName)
	fmt.Printf("User ID:   %s\n", user.Uid)
	if user.PersonalEmail.Valid {
		fmt.Printf("Email:     %s\n", user.PersonalEmail.String)
	}
	if user.Role.Valid {
		fmt.Printf("Role:      %s\n", user.Role.UserRole)
	}
	fmt.Println("---------------------------------------------------")
	fmt.Println("\nUsage with curl:")
	fmt.Printf("curl -H \"Authorization: Bearer %s\" http://localhost:8080/api/v1/auth/me\n", token)
	fmt.Println("\nToken:")
	fmt.Println(token)
}
