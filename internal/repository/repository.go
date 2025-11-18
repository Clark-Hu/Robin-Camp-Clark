package repository

import (
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Clark-Hu/Robin-Camp-Clark/internal/store"
)

// ErrNotFound indicates the requested entity does not exist.
var ErrNotFound = errors.New("repository: not found")

// Repository aggregates all domain-specific repositories.
type Repository struct {
	Movies  *MoviesRepository
	Ratings *RatingsRepository
}

// New constructs a Repository backed by the provided store.
func New(st *store.Store) *Repository {
	pool := st.Pool()
	return &Repository{
		Movies:  &MoviesRepository{pool: pool},
		Ratings: &RatingsRepository{pool: pool},
	}
}

// NewWithPool allows constructing repositories directly from a pgx pool.
func NewWithPool(pool *pgxpool.Pool) *Repository {
	return &Repository{
		Movies:  &MoviesRepository{pool: pool},
		Ratings: &RatingsRepository{pool: pool},
	}
}
