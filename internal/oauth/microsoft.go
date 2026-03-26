package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/microsoft"
)

// MicrosoftProvider handles Microsoft Entra ID (Azure AD) OAuth 2.0 authentication
type MicrosoftProvider struct {
	config *oauth2.Config
}

// MicrosoftUserInfo represents the user data returned by Microsoft
type MicrosoftUserInfo struct {
	ID                string `json:"id"`
	DisplayName       string `json:"displayName"`
	GivenName         string `json:"givenName"`
	Surname           string `json:"surname"`
	UserPrincipalName string `json:"userPrincipalName"` // Usually the email
	Mail              string `json:"mail"`
}

// NewMicrosoftProvider creates a new Microsoft OAuth provider
func NewMicrosoftProvider(clientID, clientSecret, redirectURL, tenantID string) *MicrosoftProvider {
	// If no tenant specified, use "common" for multi-tenant apps
	if tenantID == "" {
		tenantID = "common"
	}

	config := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Scopes: []string{
			"openid",
			"profile",
			"email",
			"User.Read",
		},
		Endpoint: microsoft.AzureADEndpoint(tenantID),
	}

	return &MicrosoftProvider{config: config}
}

// GetAuthURL generates the OAuth authorization URL with state token
// If redirectURLOverride is not empty, it will be used instead of the default config
func (p *MicrosoftProvider) GetAuthURL(state string, redirectURLOverride string) string {
	opts := []oauth2.AuthCodeOption{
		oauth2.AccessTypeOffline,
		oauth2.ApprovalForce,
	}
	if redirectURLOverride != "" {
		opts = append(opts, oauth2.SetAuthURLParam("redirect_uri", redirectURLOverride))
	}
	return p.config.AuthCodeURL(state, opts...)
}

// ExchangeCode exchanges the authorization code for a token and fetches user info
func (p *MicrosoftProvider) ExchangeCode(ctx context.Context, code string) (*MicrosoftUserInfo, error) {
	token, err := p.config.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code: %w", err)
	}

	// Fetch user info from Microsoft Graph API
	client := p.config.Client(ctx, token)
	resp, err := client.Get("https://graph.microsoft.com/v1.0/me")
	if err != nil {
		return nil, fmt.Errorf("failed to get user info: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("graph api request failed with status %d: %s", resp.StatusCode, body)
	}

	var userInfo MicrosoftUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return nil, fmt.Errorf("failed to decode user info: %w", err)
	}

	return &userInfo, nil
}
