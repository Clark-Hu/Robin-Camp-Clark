package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Robin-Camp/Robin-Camp/internal/boxoffice"
	"github.com/Robin-Camp/Robin-Camp/internal/config"
	"github.com/Robin-Camp/Robin-Camp/internal/repository"
)

// fakeBoxOffice returns a stub client for handler tests.
type fakeBoxOffice struct{}

func (f fakeBoxOffice) Fetch(ctx context.Context, title string) (*boxoffice.Result, error) {
	return nil, boxoffice.ErrNotFound
}

func buildTestServer(tb testing.TB) *Server {
	tb.Helper()
	cfg := config.Config{
		Port:                 "0",
		AuthToken:            "secret",
		ReadTimeoutSecs:      15,
		WriteTimeoutSecs:     15,
		IdleTimeoutSecs:      60,
		BoxOfficeTimeoutSecs: 1,
	}

	pool, cleanup := newTestPool(tb)
	tb.Cleanup(cleanup)

	repo := repository.NewWithPool(pool)
	logger := log.New(io.Discard, "", 0)
	srv := New(cfg, nil, repo, fakeBoxOffice{}, logger)
	// Replace chi router to avoid default middleware noise.
	router := chi.NewRouter()
	srv.router = router
	srv.registerRoutes()
	return srv
}

func newTestPool(tb testing.TB) (*pgxpool.Pool, func()) {
	tb.Helper()

	ctx := context.Background()

	baseDir := tb.TempDir()
	runtimeDir := filepath.Join(baseDir, "runtime")
	dataDir := filepath.Join(baseDir, "data")
	cacheDir := filepath.Join(baseDir, "cache")
	_ = os.Mkdir(runtimeDir, 0o755)
	_ = os.Mkdir(dataDir, 0o755)
	_ = os.Mkdir(cacheDir, 0o755)
	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
	port := 42000 + rnd.Intn(2000)

	db := embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().
		Username("postgres").
		Password("postgres").
		Database("movies_test_handlers").
		Port(uint32(port)).
		DataPath(dataDir).
		RuntimePath(runtimeDir).
		CachePath(cacheDir).
		Logger(io.Discard))

	if err := db.Start(); err != nil {
		tb.Fatalf("start embedded postgres: %v", err)
	}

	dsn := fmt.Sprintf("postgres://postgres:postgres@localhost:%d/movies_test_handlers?sslmode=disable", port)
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		db.Stop()
		tb.Fatalf("connect pg: %v", err)
	}

	_, currentFile, _, _ := runtime.Caller(0)
	projectRoot := filepath.Join(filepath.Dir(currentFile), "..", "..")
	migrationFiles, err := filepath.Glob(filepath.Join(projectRoot, "db", "migrations", "*_*.up.sql"))
	if err != nil {
		db.Stop()
		tb.Fatalf("list migrations: %v", err)
	}
	sort.Strings(migrationFiles)
	for _, path := range migrationFiles {
		payload, err := os.ReadFile(path)
		if err != nil {
			db.Stop()
			tb.Fatalf("read migration %s: %v", path, err)
		}
		if _, err := pool.Exec(ctx, string(payload)); err != nil {
			db.Stop()
			tb.Fatalf("apply migration %s: %v", path, err)
		}
	}

	cleanup := func() {
		pool.Close()
		_ = db.Stop()
	}
	return pool, cleanup
}

func TestHandleCreateMovie_AuthValidation(t *testing.T) {
	srv := buildTestServer(t)

	body := `{"title":"Test","genre":"Action","releaseDate":"2024-01-01"}`
	req := httptest.NewRequest(http.MethodPost, "/movies", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()

	srv.handleCreateMovie(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestHandleSubmitRating_InvalidRating(t *testing.T) {
	srv := buildTestServer(t)

	// Precreate movie
	_, err := srv.repo.Movies.Create(context.Background(), repository.MovieCreateParams{
		Title:       "Test",
		Genre:       "Action",
		ReleaseDate: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("create movie: %v", err)
	}

	payload, _ := json.Marshal(map[string]float32{"rating": 6.0})
	req := httptest.NewRequest(http.MethodPost, "/movies/Test/ratings", bytes.NewBuffer(payload))
	req.Header.Set("X-Rater-Id", "user1")
	req = attachTitleParam(req, "Test")
	rec := httptest.NewRecorder()

	srv.handleSubmitRating(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
}

func TestHandleCreateMovie_InvalidPayload(t *testing.T) {
	srv := buildTestServer(t)

	// Invalid JSON
	req := httptest.NewRequest(http.MethodPost, "/movies", bytes.NewBufferString("invalid json"))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	srv.handleCreateMovie(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422 (invalid json)", rec.Code)
	}

	// Missing required fields
	req2 := httptest.NewRequest(http.MethodPost, "/movies", bytes.NewBufferString(`{"title":"","genre":"","releaseDate":""}`))
	req2.Header.Set("Authorization", "Bearer secret")
	rec2 := httptest.NewRecorder()
	srv.handleCreateMovie(rec2, req2)
	if rec2.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422 (missing fields)", rec2.Code)
	}
}

func TestHandleListMovies_InvalidYear(t *testing.T) {
	srv := buildTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/movies?year=abc", nil)
	rec := httptest.NewRecorder()

	srv.handleListMovies(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHandleGetRating_NotFound(t *testing.T) {
	srv := buildTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/movies/Nope/rating", nil)
	req = attachTitleParam(req, "Nope")
	rec := httptest.NewRecorder()

	srv.handleGetRating(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func attachTitleParam(req *http.Request, title string) *http.Request {
	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("title", title)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, ctx))
}
