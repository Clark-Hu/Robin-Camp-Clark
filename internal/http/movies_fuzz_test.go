package httpserver

import (
	"net/url"
	"testing"
)

func FuzzBuildMovieFilters(f *testing.F) {
	seeds := []string{
		"q=Inception&genre=Action&year=2010",
		"year=abc",
		"limit=200",
		"",
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, raw string) {
		values, err := url.ParseQuery(raw)
		if err != nil {
			return
		}
		_, _ = buildMovieFilters(values)
	})
}
