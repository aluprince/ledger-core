// Package config loads and validates environment configuration.
// The app refuses to start if required values are missing.
// No defaults for secrets — explicit is safer.
package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL string
	Port        string
	JWTSecret   string
	Env         string
}

// Load reads .env (if present) then validates required env vars.
func Load() (*Config, error) {
	// .env is optional — in production, vars come from the environment directly.
	_ = godotenv.Load()

	cfg := &Config{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		Port:        os.Getenv("PORT"),
		JWTSecret:   os.Getenv("JWT_SECRET"),
		Env:         os.Getenv("APP_ENV"),
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) validate() error {
	if c.DatabaseURL == "" {
		return fmt.Errorf("config: DATABASE_URL is required")
	}
	if c.JWTSecret == "" {
		return fmt.Errorf("config: JWT_SECRET is required")
	}
	if c.Port == "" {
		c.Port = "8080"
	}
	if c.Env == "" {
		c.Env = "development"
	}
	return nil
}

func (c *Config) IsProd() bool {
	return c.Env == "production"
}
