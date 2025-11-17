package config

import (
	"strings"
	"testing"
)

func setRequiredEnvs(t *testing.T) {
	t.Helper()
	t.Setenv("AUTH_TOKEN", "secret")
	t.Setenv("DB_URL", "postgres://user:pass@localhost:5432/db")
	t.Setenv("BOXOFFICE_URL", "https://example.com/mock")
	t.Setenv("BOXOFFICE_API_KEY", "apikey")
}

func TestLoadSuccess(t *testing.T) {
	setRequiredEnvs(t)
	t.Setenv("PORT", "9090")
	t.Setenv("SERVER_READ_TIMEOUT", "30")
	t.Setenv("DB_MAX_CONNS", "40")
	t.Setenv("DB_MIN_CONNS", "5")
	t.Setenv("DB_STATEMENT_CACHE_CAPACITY", "128")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	if cfg.Port != "9090" {
		t.Fatalf("Port = %s, want 9090", cfg.Port)
	}
	if cfg.ReadTimeoutSecs != 30 {
		t.Fatalf("ReadTimeoutSecs = %d, want 30", cfg.ReadTimeoutSecs)
	}
	if cfg.DBMaxConns != 40 {
		t.Fatalf("DBMaxConns = %d, want 40", cfg.DBMaxConns)
	}
	if cfg.DBMinConns != 5 {
		t.Fatalf("DBMinConns = %d, want 5", cfg.DBMinConns)
	}
	if cfg.DBStatementCache != 128 {
		t.Fatalf("DBStatementCache = %d, want 128", cfg.DBStatementCache)
	}
}

func TestLoadValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T)
		wantErr string
	}{
		{
			name: "missing auth token",
			setup: func(t *testing.T) {
				setRequiredEnvs(t)
				t.Setenv("AUTH_TOKEN", "")
			},
			wantErr: "AUTH_TOKEN",
		},
		{
			name: "missing db url",
			setup: func(t *testing.T) {
				setRequiredEnvs(t)
				t.Setenv("DB_URL", "")
			},
			wantErr: "DB_URL",
		},
		{
			name: "negative timeout",
			setup: func(t *testing.T) {
				setRequiredEnvs(t)
				t.Setenv("BOXOFFICE_TIMEOUT_SECS", "-1")
			},
			wantErr: "BOXOFFICE_TIMEOUT_SECS",
		},
		{
			name: "min greater than max connections",
			setup: func(t *testing.T) {
				setRequiredEnvs(t)
				t.Setenv("DB_MAX_CONNS", "5")
				t.Setenv("DB_MIN_CONNS", "10")
			},
			wantErr: "DB_MIN_CONNS",
		},
		{
			name: "negative statement cache",
			setup: func(t *testing.T) {
				setRequiredEnvs(t)
				t.Setenv("DB_STATEMENT_CACHE_CAPACITY", "-1")
			},
			wantErr: "DB_STATEMENT_CACHE_CAPACITY",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup(t)
			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Load() error = %v, want contains %q", err, tt.wantErr)
			}
		})
	}
}
