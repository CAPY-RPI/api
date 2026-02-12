package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/capyrpi/api/internal/auth/ports"
	"github.com/capyrpi/api/internal/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/crypto/bcrypt"
)

type AuthResult struct {
	User  database.User
	Token string
}

type BotTokenResult struct {
	Token    database.BotToken
	RawToken string
}

type AuthService struct {
	userRepo      ports.UserRepo
	botRepo       ports.BotRepo
	tokenProvider ports.TokenProvider
	googleAuth    ports.OAuthProvider
	microsoftAuth ports.OAuthProvider
}

func NewAuthService(
	userRepo ports.UserRepo,
	botRepo ports.BotRepo,
	tokenProvider ports.TokenProvider,
	googleAuth ports.OAuthProvider,
	microsoftAuth ports.OAuthProvider,
) *AuthService {
	return &AuthService{
		userRepo:      userRepo,
		botRepo:       botRepo,
		tokenProvider: tokenProvider,
		googleAuth:    googleAuth,
		microsoftAuth: microsoftAuth,
	}
}

func (s *AuthService) HandleOAuthCallback(ctx context.Context, providerName string, code string) (*AuthResult, error) {
	var provider ports.OAuthProvider
	switch providerName {
	case "google":
		provider = s.googleAuth
	case "microsoft":
		provider = s.microsoftAuth
	default:
		return nil, errors.New("invalid provider")
	}

	userInfo, err := provider.ExchangeCode(ctx, code)
	if err != nil {
		return nil, err
	}

	pgEmail := pgtype.Text{String: userInfo.Email, Valid: true}
	user, err := s.userRepo.GetUserByEmail(ctx, pgEmail)
	if err != nil {
		if err != pgx.ErrNoRows {
			return nil, err
		}

		// Create user if not exists
		user, err = s.userRepo.CreateUser(ctx, database.CreateUserParams{
			FirstName:     userInfo.FirstName,
			LastName:      userInfo.LastName,
			PersonalEmail: pgEmail,
			SchoolEmail:   pgtype.Text{Valid: false},
			Role:          database.NullUserRole{UserRole: database.UserRoleStudent, Valid: true},
		})
		if err != nil {
			return nil, err
		}
	}

	token, err := s.tokenProvider.GenerateToken(user)
	if err != nil {
		return nil, err
	}

	return &AuthResult{User: user, Token: token}, nil
}

func (s *AuthService) GenerateBotToken(ctx context.Context, name string, createdBy uuid.UUID, expiresAt *time.Time) (*BotTokenResult, error) {
	rawToken, err := generateSecureToken(32)
	if err != nil {
		return nil, err
	}

	hashedToken, err := bcrypt.GenerateFromPassword([]byte(rawToken), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	pgExpiresAt := pgtype.Timestamp{Valid: false}
	if expiresAt != nil {
		pgExpiresAt = pgtype.Timestamp{Time: *expiresAt, Valid: true}
	}

	token, err := s.botRepo.CreateBotToken(ctx, database.CreateBotTokenParams{
		TokenHash: string(hashedToken),
		Name:      name,
		CreatedBy: createdBy,
		ExpiresAt: pgExpiresAt,
	})
	if err != nil {
		return nil, err
	}

	return &BotTokenResult{Token: token, RawToken: rawToken}, nil
}

func generateSecureToken(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
