package repository

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"testing"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Robin-Camp/Robin-Camp/internal/domain"
)

type testEnv struct {
	ctx        context.Context
	pool       *pgxpool.Pool
	repository *Repository
	postgres   *embeddedpostgres.EmbeddedPostgres
}

func newTestEnv(t testing.TB) *testEnv {
	t.Helper()

	ctx := context.Background()

	baseDir := t.TempDir()
	runtimeDir := filepath.Join(baseDir, "runtime")
	dataDir := filepath.Join(baseDir, "data")
	cacheDir := filepath.Join(baseDir, "cache")
	_ = os.Mkdir(runtimeDir, 0o755)
	_ = os.Mkdir(dataDir, 0o755)
	_ = os.Mkdir(cacheDir, 0o755)
	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
	port := 40000 + rnd.Intn(2000)

	db := embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().
		Username("postgres").
		Password("postgres").
		Database("movies_test").
		Port(uint32(port)).
		DataPath(dataDir).
		RuntimePath(runtimeDir).
		CachePath(cacheDir).
		Logger(io.Discard))

	if err := db.Start(); err != nil {
		t.Fatalf("start embedded postgres: %v", err)
	}

	dsn := fmt.Sprintf("postgres://postgres:postgres@localhost:%d/movies_test?sslmode=disable", port)
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		db.Stop()
		t.Fatalf("connect pg: %v", err)
	}

	_, currentFile, _, _ := runtime.Caller(0)
	projectRoot := filepath.Join(filepath.Dir(currentFile), "..", "..")
	migrationFiles, err := filepath.Glob(filepath.Join(projectRoot, "db", "migrations", "*_*.up.sql"))
	if err != nil {
		db.Stop()
		t.Fatalf("list migrations: %v", err)
	}
	if len(migrationFiles) == 0 {
		db.Stop()
		t.Fatalf("no migration files found")
	}
	sort.Strings(migrationFiles)
	for _, path := range migrationFiles {
		payload, err := os.ReadFile(path)
		if err != nil {
			db.Stop()
			t.Fatalf("read migration %s: %v", path, err)
		}
		if _, err := pool.Exec(ctx, string(payload)); err != nil {
			db.Stop()
			t.Fatalf("apply migration %s: %v", path, err)
		}
	}

	return &testEnv{
		ctx:        ctx,
		postgres:   db,
		pool:       pool,
		repository: NewWithPool(pool),
	}
}

func (e *testEnv) cleanup() {
	if e.pool != nil {
		e.pool.Close()
	}
	if e.postgres != nil {
		_ = e.postgres.Stop()
	}
}

func mustCreateMovie(t testing.TB, env *testEnv, title string) domain.Movie {
	t.Helper()
	params := MovieCreateParams{
		Title:       title,
		ReleaseDate: time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC),
		Genre:       "Action",
	}
	movie, err := env.repository.Movies.Create(env.ctx, params)
	if err != nil {
		t.Fatalf("create movie %q: %v", title, err)
	}
	return movie
}

func TestMoviesRepository_CreateGetList(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()

	boxOffice := &domain.BoxOffice{
		Revenue:     domain.Revenue{Worldwide: 100_000},
		Currency:    "USD",
		Source:      "test",
		LastUpdated: time.Now().UTC(),
	}

	movieA := mustCreateMovie(t, env, "Movie A")
	_, err := env.repository.Movies.UpdateMetadata(env.ctx, movieA.ID, nil, nil, nil, boxOffice)
	if err != nil {
		t.Fatalf("update metadata: %v", err)
	}

	movieB := mustCreateMovie(t, env, "Movie B")

	gotByTitle, err := env.repository.Movies.GetByTitle(env.ctx, "Movie A")
	if err != nil {
		t.Fatalf("GetByTitle: %v", err)
	}
	if gotByTitle.BoxOffice == nil || gotByTitle.BoxOffice.Currency != "USD" {
		t.Fatalf("BoxOffice not loaded correctly: %+v", gotByTitle.BoxOffice)
	}

	if _, err := env.repository.Movies.GetByID(env.ctx, "non-existent"); err == nil {
		t.Fatalf("expected ErrNotFound for unknown ID")
	}

	filters := MovieListFilters{Limit: 1}
	firstPage, err := env.repository.Movies.List(env.ctx, filters)
	if err != nil {
		t.Fatalf("List first page: %v", err)
	}
	if len(firstPage.Items) != 1 {
		t.Fatalf("first page size = %d, want 1", len(firstPage.Items))
	}
	if firstPage.NextCursor == nil {
		t.Fatalf("expected next cursor")
	}

	cursor, err := DecodeCursor(*firstPage.NextCursor)
	if err != nil {
		t.Fatalf("decode cursor: %v", err)
	}

	filters.Cursor = cursor
	secondPage, err := env.repository.Movies.List(env.ctx, filters)
	if err != nil {
		t.Fatalf("List second page: %v", err)
	}
	if len(secondPage.Items) != 1 {
		t.Fatalf("second page size = %d, want 1", len(secondPage.Items))
	}
	if firstPage.Items[0].ID == secondPage.Items[0].ID {
		t.Fatalf("pagination returned duplicate movie")
	}

	// Sanity check GetByID on second movie.
	gotByID, err := env.repository.Movies.GetByID(env.ctx, movieB.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if gotByID.Title != movieB.Title {
		t.Fatalf("GetByID title = %s, want %s", gotByID.Title, movieB.Title)
	}
}

func TestRatingsRepository_UpsertAndAggregate(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()

	movie := mustCreateMovie(t, env, "Rating Movie")

	params := RatingUpsertParams{
		MovieID: movie.ID,
		RaterID: "user1",
		Value:   4.5,
	}
	rating, inserted, err := env.repository.Ratings.Upsert(env.ctx, params)
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	if !inserted {
		t.Fatalf("expected first upsert to insert")
	}
	if rating.Value != params.Value {
		t.Fatalf("rating value = %v, want %v", rating.Value, params.Value)
	}

	params.Value = 3.5
	_, inserted, err = env.repository.Ratings.Upsert(env.ctx, params)
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	if inserted {
		t.Fatalf("expected update, not insert")
	}

	// Add another rater
	_, inserted, err = env.repository.Ratings.Upsert(env.ctx, RatingUpsertParams{
		MovieID: movie.ID,
		RaterID: "user2",
		Value:   4.0,
	})
	if err != nil {
		t.Fatalf("third upsert: %v", err)
	}
	if !inserted {
		t.Fatalf("expected insert for second rater")
	}

	agg, err := env.repository.Ratings.Aggregate(env.ctx, movie.ID)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	if agg.Count != 2 {
		t.Fatalf("agg count = %d, want 2", agg.Count)
	}
	if agg.Average < 3.7 || agg.Average > 3.8 {
		t.Fatalf("agg average = %v, want around 3.8", agg.Average)
	}

	fetched, err := env.repository.Ratings.Get(env.ctx, movie.ID, "user1")
	if err != nil {
		t.Fatalf("get rating: %v", err)
	}
	if fetched.Value != 3.5 {
		t.Fatalf("fetched rating = %v, want 3.5", fetched.Value)
	}

	if _, err := env.repository.Ratings.Get(env.ctx, movie.ID, "missing"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound for missing rating, got %v", err)
	}
}

func TestRatingsRepository_AggregateEmpty(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()

	movie := mustCreateMovie(t, env, "No Ratings Movie")

	agg, err := env.repository.Ratings.Aggregate(env.ctx, movie.ID)
	if err != nil {
		t.Fatalf("aggregate without ratings: %v", err)
	}
	if agg.Count != 0 {
		t.Fatalf("agg.Count = %d, want 0", agg.Count)
	}
	if agg.Average != 0 {
		t.Fatalf("agg.Average = %v, want 0", agg.Average)
	}
}

func TestRatingsRepository_ConcurrentUpserts(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()

	movie := mustCreateMovie(t, env, "Concurrent Movie")
	const workers = 10
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		rater := fmt.Sprintf("user-%d", i)
		wg.Add(1)
		go func(rater string) {
			defer wg.Done()
			params := RatingUpsertParams{
				MovieID: movie.ID,
				RaterID: rater,
				Value:   4.0,
			}
			if _, inserted, err := env.repository.Ratings.Upsert(env.ctx, params); err != nil {
				t.Errorf("upsert failed for %s: %v", rater, err)
			} else if !inserted {
				t.Errorf("expected insert for %s", rater)
			}
		}(rater)
	}
	wg.Wait()

	agg, err := env.repository.Ratings.Aggregate(env.ctx, movie.ID)
	if err != nil {
		t.Fatalf("aggregate after concurrent upserts: %v", err)
	}
	if agg.Count != workers {
		t.Fatalf("agg.Count = %d, want %d", agg.Count, workers)
	}
}

func BenchmarkMoviesRepositoryCreate(b *testing.B) {
	env := newTestEnv(b)
	defer env.cleanup()

	for i := 0; i < b.N; i++ {
		title := fmt.Sprintf("Bench Movie %d", i)
		_, err := env.repository.Movies.Create(env.ctx, MovieCreateParams{
			Title:       title,
			Genre:       "Action",
			ReleaseDate: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		})
		if err != nil {
			b.Fatalf("create movie: %v", err)
		}
	}
}

func BenchmarkRatingsRepositoryUpsert(b *testing.B) {
	env := newTestEnv(b)
	defer env.cleanup()

	movie := mustCreateMovie(b, env, "Bench Movie")
	for i := 0; i < b.N; i++ {
		rater := fmt.Sprintf("bench-%d", i)
		_, _, err := env.repository.Ratings.Upsert(env.ctx, RatingUpsertParams{
			MovieID: movie.ID,
			RaterID: rater,
			Value:   4.0,
		})
		if err != nil {
			b.Fatalf("upsert: %v", err)
		}
	}
}
