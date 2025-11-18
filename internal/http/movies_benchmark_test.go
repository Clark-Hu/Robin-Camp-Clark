package httpserver

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Robin-Camp/Robin-Camp/internal/repository"
)

func BenchmarkHandleSubmitRating(b *testing.B) {
	srv := buildTestServer(b)

	movie, err := srv.repo.Movies.Create(context.Background(), repository.MovieCreateParams{
		Title:       "Benchmark Movie",
		Genre:       "Action",
		ReleaseDate: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		b.Fatalf("create movie: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		payload := []byte(`{"rating":4.0}`)
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/movies/%s/ratings", movie.ID), bytes.NewReader(payload))
		req.Header.Set("X-Rater-Id", fmt.Sprintf("bench-%d", i))
		req = attachIDParam(req, movie.ID)
		rec := httptest.NewRecorder()

		srv.handleSubmitRating(rec, req)
		if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
			b.Fatalf("unexpected status %d", rec.Code)
		}
	}
}
