package config

import (
	"errors"
	"os"
)

// Config holds runtime configuration sourced from environment variables.
type Config struct {
	Port        string
	DatabaseURL string
	RedisURL    string
	MasterKey   string
	LogLevel    string
}

// Load reads configuration from the environment and validates required values.
func Load() (*Config, error) {
	cfg := &Config{
		Port:        getEnv("PORT", "8080"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		RedisURL:    os.Getenv("REDIS_URL"),
		MasterKey:   os.Getenv("MASTER_KEY"),
		LogLevel:    getEnv("LOG_LEVEL", "info"),
	}
	if cfg.DatabaseURL == "" {
		return nil, errors.New("DATABASE_URL is required")
	}
	if cfg.MasterKey == "" {
		return nil, errors.New("MASTER_KEY is required")
	}
	if len(cfg.MasterKey) < 16 {
		return nil, errors.New("MASTER_KEY must be at least 16 characters")
	}
	return cfg, nil
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}