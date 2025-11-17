package boxoffice

import (
	"context"
	"io"
	"log"
	"os"
	"testing"
	"time"
)

// TestHTTPClientSmoke is used by scripts/monitor-boxoffice.sh to ensure
// that the HTTP client can parse at least one record from a target service.
func TestHTTPClientSmoke(t *testing.T) {
	baseURL := os.Getenv("BOXOFFICE_URL")
	if baseURL == "" {
		t.Skip("BOXOFFICE_URL not provided")
	}
	apiKey := os.Getenv("BOXOFFICE_API_KEY")
	client, err := NewHTTPClient(baseURL, apiKey, 3*time.Second, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatalf("create http client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := client.Fetch(ctx, "Inception")
	if err != nil {
		t.Fatalf("fetch mock data: %v", err)
	}
	if result.BoxOffice == nil || result.BoxOffice.Currency == "" {
		t.Fatalf("unexpected box office payload: %+v", result)
	}
}
