package config

import (
	"time"

	"github.com/caarlos0/env/v11"
)

// Config contains runtime configuration loaded from environment variables.
type Config struct {
	DatabaseURL         string        `env:"DATABASE_URL,required" envDefault:"postgres://app:app@localhost:5432/app?sslmode=disable"`
	MigrationsPath      string        `env:"MIGRATIONS_PATH" envDefault:"file://migrations"`
	DatabasePingTimeout time.Duration `env:"DATABASE_PING_TIMEOUT" envDefault:"5s"`
}

// Load reads environment variables into Config.
func Load() (Config, error) {
	cfg, err := env.ParseAs[Config]()
	if err != nil {
		return Config{}, err
	}

	return cfg, nil
}
