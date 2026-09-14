package config

import (
	"os"
	"time"
)

type AuthConfig struct {
	JWTSecret     string
	JWTIssuer     string
	JWTAccessTTL  time.Duration
	JWTRefreshTTL time.Duration
}

func LoadAuthConfig() AuthConfig {
	return AuthConfig{
		JWTSecret:     os.Getenv("JWT_SECRET"),
		JWTIssuer:     os.Getenv("JWT_ISSUER"),
		JWTAccessTTL:  parseDuration(os.Getenv("JWT_ACCESS_TTL"), 15*time.Minute),
		JWTRefreshTTL: parseDuration(os.Getenv("JWT_REFRESH_TTL"), 7*24*time.Hour),
	}
}

func parseDuration(value string, fallback time.Duration) time.Duration {
	if value == "" {
		return fallback
	}

	duration, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}

	return duration
}
