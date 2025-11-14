package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config captures all runtime configuration derived from environment variables.
type Config struct {
	Port                 string
	AuthToken            string
	DBURL                string
	BoxOfficeURL         string
	BoxOfficeAPIKey      string
	BoxOfficeTimeoutSecs int
	ReadTimeoutSecs      int
	WriteTimeoutSecs     int
	IdleTimeoutSecs      int
	DBMaxConns           int
	DBMinConns           int
	DBMaxIdleSecs        int
	DBMaxLifeSecs        int
	DBConnTimeoutSecs    int
	DBStatementCache     int
}

// Load reads configuration from environment variables, applying defaults and validation.
func Load() (Config, error) {
	cfg := Config{
		Port:                 getEnv("PORT", "8080"),
		AuthToken:            os.Getenv("AUTH_TOKEN"),
		DBURL:                os.Getenv("DB_URL"),
		BoxOfficeURL:         os.Getenv("BOXOFFICE_URL"),
		BoxOfficeAPIKey:      os.Getenv("BOXOFFICE_API_KEY"),
		BoxOfficeTimeoutSecs: getEnvInt("BOXOFFICE_TIMEOUT_SECS", 5),
		ReadTimeoutSecs:      getEnvInt("SERVER_READ_TIMEOUT", 15),
		WriteTimeoutSecs:     getEnvInt("SERVER_WRITE_TIMEOUT", 15),
		IdleTimeoutSecs:      getEnvInt("SERVER_IDLE_TIMEOUT", 60),
		DBMaxConns:           getEnvInt("DB_MAX_CONNS", 20),
		DBMinConns:           getEnvInt("DB_MIN_CONNS", 2),
		DBMaxIdleSecs:        getEnvInt("DB_MAX_CONN_IDLE_SECS", 300),
		DBMaxLifeSecs:        getEnvInt("DB_MAX_CONN_LIFETIME_SECS", 3600),
		DBConnTimeoutSecs:    getEnvInt("DB_CONN_TIMEOUT_SECS", 10),
		DBStatementCache:     getEnvInt("DB_STATEMENT_CACHE_CAPACITY", 256),
	}

	if cfg.AuthToken == "" {
		return Config{}, fmt.Errorf("AUTH_TOKEN is required")
	}
	if cfg.DBURL == "" {
		return Config{}, fmt.Errorf("DB_URL is required")
	}
	if cfg.BoxOfficeURL == "" {
		return Config{}, fmt.Errorf("BOXOFFICE_URL is required")
	}
	if cfg.BoxOfficeAPIKey == "" {
		return Config{}, fmt.Errorf("BOXOFFICE_API_KEY is required")
	}
	if cfg.BoxOfficeTimeoutSecs <= 0 {
		return Config{}, fmt.Errorf("BOXOFFICE_TIMEOUT_SECS must be positive")
	}
	if cfg.DBMaxConns <= 0 {
		return Config{}, fmt.Errorf("DB_MAX_CONNS must be positive")
	}
	if cfg.DBMinConns < 0 {
		return Config{}, fmt.Errorf("DB_MIN_CONNS must be non-negative")
	}
	if cfg.DBMaxConns > 0 && cfg.DBMinConns > cfg.DBMaxConns {
		return Config{}, fmt.Errorf("DB_MIN_CONNS cannot exceed DB_MAX_CONNS")
	}
	if cfg.DBStatementCache < 0 {
		return Config{}, fmt.Errorf("DB_STATEMENT_CACHE_CAPACITY must be non-negative")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if val := os.Getenv(key); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil {
			return parsed
		}
	}
	return fallback
}
