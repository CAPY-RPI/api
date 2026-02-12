package adapters

import (
	"context"

	"github.com/capyrpi/api/internal/auth/ports"
	"github.com/capyrpi/api/internal/oauth"
)

// GoogleOAuthAdapter wraps internal/oauth/google.go
type GoogleOAuthAdapter struct {
	provider *oauth.GoogleProvider
}

func NewGoogleOAuthAdapter(provider *oauth.GoogleProvider) ports.OAuthProvider {
	return &GoogleOAuthAdapter{
		provider: provider,
	}
}

func (a *GoogleOAuthAdapter) GetAuthURL(state string) string {
	return a.provider.GetAuthURL(state)
}

func (a *GoogleOAuthAdapter) ExchangeCode(ctx context.Context, code string) (*ports.UserInfo, error) {
	userInfo, err := a.provider.ExchangeCode(ctx, code)
	if err != nil {
		return nil, err
	}
	return &ports.UserInfo{
		Email:     userInfo.Email,
		FirstName: userInfo.GivenName,
		LastName:  userInfo.FamilyName,
	}, nil
}

// MicrosoftOAuthAdapter wraps internal/oauth/microsoft.go
type MicrosoftOAuthAdapter struct {
	provider *oauth.MicrosoftProvider
}

func NewMicrosoftOAuthAdapter(provider *oauth.MicrosoftProvider) ports.OAuthProvider {
	return &MicrosoftOAuthAdapter{
		provider: provider,
	}
}

func (a *MicrosoftOAuthAdapter) GetAuthURL(state string) string {
	return a.provider.GetAuthURL(state)
}

func (a *MicrosoftOAuthAdapter) ExchangeCode(ctx context.Context, code string) (*ports.UserInfo, error) {
	userInfo, err := a.provider.ExchangeCode(ctx, code)
	if err != nil {
		return nil, err
	}

	email := userInfo.UserPrincipalName
	if email == "" {
		email = userInfo.Mail
	}

	return &ports.UserInfo{
		Email:     email,
		FirstName: userInfo.GivenName,
		LastName:  userInfo.Surname,
	}, nil
}
