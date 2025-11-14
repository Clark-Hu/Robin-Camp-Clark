package domain

import "time"

// Revenue captures the nested revenue payload returned by the box office API.
type Revenue struct {
	Worldwide        int64  `json:"worldwide"`
	OpeningWeekendUS *int64 `json:"openingWeekendUSA,omitempty"`
}

// BoxOffice mirrors the contract's boxOffice object.
type BoxOffice struct {
	Revenue     Revenue   `json:"revenue"`
	Currency    string    `json:"currency"`
	Source      string    `json:"source"`
	LastUpdated time.Time `json:"lastUpdated"`
}

// Movie represents the canonical movie entity in the database/service.
type Movie struct {
	ID          string
	Title       string
	ReleaseDate time.Time
	ReleaseYear int
	Genre       string
	Distributor *string
	Budget      *int64
	MpaRating   *string
	BoxOffice   *BoxOffice
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
