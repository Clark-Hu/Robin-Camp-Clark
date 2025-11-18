package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Robin-Camp/Robin-Camp/internal/domain"
)

// RatingsRepository provides helpers for movie ratings.
type RatingsRepository struct {
	pool *pgxpool.Pool
}

// RatingUpsertParams captures the payload required to upsert a rating.
type RatingUpsertParams struct {
	MovieID string
	RaterID string
	Value   float32
}

// Upsert inserts or updates a rating and indicates whether it was newly created.
func (r *RatingsRepository) Upsert(ctx context.Context, params RatingUpsertParams) (domain.Rating, bool, error) {
	const query = `
        INSERT INTO ratings (movie_id, rater_id, rating)
        VALUES ($1,$2,$3)
        ON CONFLICT (movie_id, rater_id)
        DO UPDATE SET rating = EXCLUDED.rating, updated_at = now()
        RETURNING movie_id, rater_id, rating, created_at, updated_at, (xmax = 0) AS inserted
    `

	var rating domain.Rating
	var inserted bool
	err := r.pool.QueryRow(ctx, query, params.MovieID, params.RaterID, params.Value).Scan(
		&rating.MovieID,
		&rating.RaterID,
		&rating.Value,
		&rating.CreatedAt,
		&rating.UpdatedAt,
		&inserted,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return domain.Rating{}, false, ErrNotFound
		}
		return domain.Rating{}, false, err
	}

	return rating, inserted, nil
}

// Aggregate returns the rating average and count for a movie.
func (r *RatingsRepository) Aggregate(ctx context.Context, movieID string) (domain.RatingAggregate, error) {
	const query = `
        SELECT COALESCE(ROUND(AVG(rating)::numeric, 1), 0)::float4 AS average,
               COUNT(*)::int8 AS count
        FROM ratings
        WHERE movie_id = $1
    `

	var agg domain.RatingAggregate
	err := r.pool.QueryRow(ctx, query, movieID).Scan(&agg.Average, &agg.Count)
	if err != nil {
		return domain.RatingAggregate{}, fmt.Errorf("aggregate ratings: %w", err)
	}
	return agg, nil
}

// Get retrieves a rating for a specific rater/movie combination.
func (r *RatingsRepository) Get(ctx context.Context, movieID, raterID string) (domain.Rating, error) {
	const query = `
        SELECT movie_id, rater_id, rating, created_at, updated_at
        FROM ratings
        WHERE movie_id = $1 AND rater_id = $2
    `
	var rating domain.Rating
	err := r.pool.QueryRow(ctx, query, movieID, raterID).Scan(
		&rating.MovieID,
		&rating.RaterID,
		&rating.Value,
		&rating.CreatedAt,
		&rating.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return domain.Rating{}, ErrNotFound
		}
		return domain.Rating{}, err
	}
	return rating, nil
}
