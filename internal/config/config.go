package config

import (
	"time"

	"github.com/caarlos0/env/v11"
)

// Config contains runtime configuration loaded from environment variables.
type Config struct {
	// Application
	AppBaseURL string `env:"APP_BASE_URL" envDefault:"http://localhost:8080"`
	Scheme     string `env:"SCHEME" envDefault:"http"`

	// Database
	DatabaseURL         string        `env:"DATABASE_URL,required" envDefault:"postgres://app:app@localhost:5432/app?sslmode=disable"`
	MigrationsPath      string        `env:"MIGRATIONS_PATH" envDefault:"file://migrations"`
	DatabasePingTimeout time.Duration `env:"DATABASE_PING_TIMEOUT" envDefault:"5s"`

	// GitHub API
	GitHubAPIBaseURL string        `env:"GITHUB_API_BASE_URL" envDefault:"https://api.github.com"`
	GitHubAuthToken  string        `env:"GITHUB_AUTH_TOKEN"`
	GitHubAPITimeout time.Duration `env:"GITHUB_API_TIMEOUT" envDefault:"5s"`

	// SMTP
	SMTPHost     string `env:"SMTP_HOST" envDefault:"localhost"`
	SMTPPort     int    `env:"SMTP_PORT" envDefault:"1025"`
	SMTPFrom     string `env:"SMTP_FROM" envDefault:"no-reply@github-release-notification.local"`
	SMTPUsername string `env:"SMTP_USERNAME"`
	SMTPPassword string `env:"SMTP_PASSWORD"`

	// Scanner
	ScannerInterval time.Duration `env:"SCANNER_INTERVAL" envDefault:"1m"`

	// Notifier
	NotifierInterval time.Duration `env:"NOTIFIER_INTERVAL" envDefault:"1m"`
}

// Load reads environment variables into Config.
func Load() (Config, error) {
	cfg, err := env.ParseAs[Config]()
	if err != nil {
		return Config{}, err
	}

	return cfg, nil
}
