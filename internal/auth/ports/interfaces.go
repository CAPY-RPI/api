package ports

import (
	"context"

	"github.com/capyrpi/api/internal/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// UserInfo represents the normalized user information from OAuth providers
type UserInfo struct {
	Email     string
	FirstName string
	LastName  string
}

// UserRepo defines the interface for user persistence
type UserRepo interface {
	GetUserByEmail(ctx context.Context, email pgtype.Text) (database.User, error)
	CreateUser(ctx context.Context, arg database.CreateUserParams) (database.User, error)
	GetUserByID(ctx context.Context, uid uuid.UUID) (database.User, error)
}

// TokenProvider defines the interface for token generation
type TokenProvider interface {
	GenerateToken(user database.User) (string, error)
}

// OAuthProvider defines the interface for OAuth operations
type OAuthProvider interface {
	GetAuthURL(state string) string
	ExchangeCode(ctx context.Context, code string) (*UserInfo, error)
}
