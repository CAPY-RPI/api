package service

import (
	"github.com/capyrpi/api/internal/auth/ports"
)

type AuthService struct {
	userRepo      ports.UserRepo
	tokenProvider ports.TokenProvider
	googleAuth    ports.OAuthProvider
	microsoftAuth ports.OAuthProvider
}

func NewAuthService(
	userRepo ports.UserRepo,
	tokenProvider ports.TokenProvider,
	googleAuth ports.OAuthProvider,
	microsoftAuth ports.OAuthProvider,
) *AuthService {
	return &AuthService{
		userRepo:      userRepo,
		tokenProvider: tokenProvider,
		googleAuth:    googleAuth,
		microsoftAuth: microsoftAuth,
	}
}
