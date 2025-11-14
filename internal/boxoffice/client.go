package boxoffice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Robin-Camp/Robin-Camp/internal/domain"
)

// ErrNotFound is returned when upstream cannot find the requested movie.
var ErrNotFound = errors.New("boxoffice: not found")

// Result contains the data required to enrich a movie record.
type Result struct {
	Distributor *string
	Budget      *int64
	MpaRating   *string
	BoxOffice   *domain.BoxOffice
}

// Client defines the contract for querying the upstream box office API.
type Client interface {
	Fetch(ctx context.Context, title string) (*Result, error)
}

// HTTPClient implements Client over HTTP.
type HTTPClient struct {
	baseURL *url.URL
	apiKey  string
	client  *http.Client
	logger  *log.Logger
}

// NewHTTPClient constructs a new HTTP-backed box office client.
func NewHTTPClient(baseURL, apiKey string, timeout time.Duration, logger *log.Logger) (*HTTPClient, error) {
	if logger == nil {
		logger = log.Default()
	}
	parsed, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil {
		return nil, fmt.Errorf("parse box office url: %w", err)
	}
	return &HTTPClient{
		baseURL: parsed,
		apiKey:  apiKey,
		client: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
				DialContext: (&net.Dialer{
					Timeout:   timeout,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				TLSHandshakeTimeout:   timeout,
				ResponseHeaderTimeout: timeout,
				ExpectContinueTimeout: 1 * time.Second,
			},
		},
		logger: logger,
	}, nil
}

// Fetch retrieves box office information by title.
func (c *HTTPClient) Fetch(ctx context.Context, title string) (*Result, error) {
	rel := &url.URL{Path: "/boxoffice"}
	q := rel.Query()
	q.Set("title", title)
	rel.RawQuery = q.Encode()
	endpoint := c.baseURL.ResolveReference(rel)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-API-Key", c.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		var payload apiResponse
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			return nil, fmt.Errorf("decode box office response: %w", err)
		}
		return convertToResult(payload), nil
	case http.StatusNotFound:
		return nil, ErrNotFound
	default:
		c.logger.Printf("boxoffice: unexpected status %d for title %q", resp.StatusCode, title)
		return nil, fmt.Errorf("boxoffice: upstream returned %d", resp.StatusCode)
	}
}

type apiResponse struct {
	Title       string         `json:"title"`
	Distributor *string        `json:"distributor"`
	ReleaseDate *string        `json:"releaseDate"`
	Budget      *int64         `json:"budget"`
	Revenue     revenuePayload `json:"revenue"`
	MpaRating   *string        `json:"mpaRating"`
	Currency    *string        `json:"currency"`
	Source      *string        `json:"source"`
	LastUpdated *time.Time     `json:"lastUpdated"`
}

type revenuePayload struct {
	Worldwide        *int64 `json:"worldwide"`
	OpeningWeekendUS *int64 `json:"openingWeekendUSA"`
}

func convertToResult(payload apiResponse) *Result {
	if payload.Revenue.Worldwide == nil {
		zero := int64(0)
		payload.Revenue.Worldwide = &zero
	}

	lastUpdated := time.Now().UTC()
	if payload.LastUpdated != nil {
		lastUpdated = payload.LastUpdated.UTC()
	}

	currency := "USD"
	if payload.Currency != nil && *payload.Currency != "" {
		currency = *payload.Currency
	}
	source := "BoxOfficeAPI"
	if payload.Source != nil && *payload.Source != "" {
		source = *payload.Source
	}

	boxOffice := &domain.BoxOffice{
		Revenue: domain.Revenue{
			Worldwide:        derefInt64(payload.Revenue.Worldwide),
			OpeningWeekendUS: payload.Revenue.OpeningWeekendUS,
		},
		Currency:    currency,
		Source:      source,
		LastUpdated: lastUpdated,
	}

	return &Result{
		Distributor: payload.Distributor,
		Budget:      payload.Budget,
		MpaRating:   payload.MpaRating,
		BoxOffice:   boxOffice,
	}
}

func derefInt64(ptr *int64) int64 {
	if ptr == nil {
		return 0
	}
	return *ptr
}
