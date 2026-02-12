package adapters

import (
	"time"

	"github.com/capyrpi/api/internal/auth/ports"
	"github.com/capyrpi/api/internal/config"
	"github.com/capyrpi/api/internal/database"
	"github.com/capyrpi/api/internal/middleware"
	"github.com/golang-jwt/jwt/v5"
)

type JWTAdapter struct {
	cfg *config.Config
}

func NewJWTAdapter(cfg *config.Config) ports.TokenProvider {
	return &JWTAdapter{
		cfg: cfg,
	}
}

func (a *JWTAdapter) GenerateToken(user database.User) (string, error) {
	claims := &middleware.UserClaims{
		UserID: user.Uid.String(),
		Email:  getEmail(user),
		Role:   string(user.Role.UserRole),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(a.cfg.JWT.ExpiryHours) * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "capy-api",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(a.cfg.JWT.Secret))
}

func getEmail(user database.User) string {
	if user.SchoolEmail.Valid {
		return user.SchoolEmail.String
	}
	if user.PersonalEmail.Valid {
		return user.PersonalEmail.String
	}
	return ""
}
