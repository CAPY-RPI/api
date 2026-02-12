package config

import (
	"github.com/ilyakaznacheev/cleanenv"
)

type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	JWT      JWTConfig
	Cookie   CookieConfig
	OAuth    OAuthConfig
	Swagger  SwaggerConfig
	Env      string `env:"ENV" env-default:"development"`
}

type SwaggerConfig struct {
	Host string `env:"SWAGGER_HOST"`
}

type ServerConfig struct {
	Host           string   `env:"SERVER_HOST" env-default:"0.0.0.0"`
	Port           string   `env:"SERVER_PORT" env-default:"8080"`
	AllowedOrigins []string `env:"ALLOWED_ORIGINS" env-default:"http://localhost:3000,http://localhost:8080" env-separator:","`
}

type DatabaseConfig struct {
	URL        string `env:"DATABASE_URL" env-required:"true"`
	SchemaPath string `env:"SCHEMA_PATH" env-default:"schema.sql"`
}

type JWTConfig struct {
	Secret      string `env:"JWT_SECRET" env-required:"true"`
	ExpiryHours int    `env:"JWT_EXPIRY_HOURS" env-default:"24"`
}

type CookieConfig struct {
	Domain string `env:"COOKIE_DOMAIN" env-default:"localhost"`
	Secure bool   `env:"COOKIE_SECURE" env-default:"false"`
}

type OAuthConfig struct {
	Google      GoogleOAuthConfig
	Microsoft   MicrosoftOAuthConfig
	RedirectURL string `env:"AUTH_REDIRECT_URL" env-default:"/"`
}

type GoogleOAuthConfig struct {
	ClientID     string `env:"GOOGLE_CLIENT_ID"`
	ClientSecret string `env:"GOOGLE_CLIENT_SECRET"`
	RedirectURL  string `env:"GOOGLE_REDIRECT_URL"`
}

type MicrosoftOAuthConfig struct {
	ClientID     string `env:"MICROSOFT_CLIENT_ID"`
	ClientSecret string `env:"MICROSOFT_CLIENT_SECRET"`
	TenantID     string `env:"MICROSOFT_TENANT_ID" env-default:"common"`
	RedirectURL  string `env:"MICROSOFT_REDIRECT_URL"`
}

// Load reads configuration from environment variables
func Load() (*Config, error) {
	var cfg Config
	if err := cleanenv.ReadEnv(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
