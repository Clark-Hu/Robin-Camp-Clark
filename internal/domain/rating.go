package domain

import "time"

// Rating represents a single user's rating for a movie.
type Rating struct {
	MovieID   string
	RaterID   string
	Value     float32
	CreatedAt time.Time
	UpdatedAt time.Time
}

// RatingAggregate provides average and count for a movie's ratings.
type RatingAggregate struct {
	Average float32
	Count   int64
}
