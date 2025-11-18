package httpserver

import (
	"net/url"
	"testing"

	"github.com/Robin-Camp/Robin-Camp/internal/config"
)

func TestBuildMovieFilters(t *testing.T) {
	values, _ := url.ParseQuery("q= Nolan &year=2010&genre=Action&distributor= Warner &budget=100000000&mpaRating=PG-13&limit=150")

	filters, err := buildMovieFilters(values)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if filters.Query == nil || *filters.Query != "Nolan" {
		t.Fatalf("query not trimmed: %+v", filters.Query)
	}
	if filters.Year == nil || *filters.Year != 2010 {
		t.Fatalf("year parse failed: %+v", filters.Year)
	}
	if filters.Genre == nil || *filters.Genre != "Action" {
		t.Fatalf("genre parse failed: %+v", filters.Genre)
	}
	if filters.Distributor == nil || *filters.Distributor != "Warner" {
		t.Fatalf("distributor parse failed")
	}
	if filters.BudgetLTE == nil || *filters.BudgetLTE != 100000000 {
		t.Fatalf("budget parse failed")
	}
	if filters.MpaRating == nil || *filters.MpaRating != "PG-13" {
		t.Fatalf("mpa rating parse failed")
	}
	if filters.Limit != 150 {
		t.Fatalf("limit not parsed: %d", filters.Limit)
	}
}

func TestBuildMovieFilters_InvalidYear(t *testing.T) {
	values, _ := url.ParseQuery("year=abc")
	if _, err := buildMovieFilters(values); err == nil {
		t.Fatalf("expected error for invalid year")
	}
}

func TestVerifyBearer(t *testing.T) {
	srv := &Server{cfg: config.Config{AuthToken: "secret"}}
	cases := []struct {
		header  string
		allowed bool
	}{
		{"Bearer secret", true},
		{"Bearer secret ", true},
		{"Bearer other", false},
		{"secret", false},
		{"", false},
	}
	for _, c := range cases {
		if srv.verifyBearer(c.header) != c.allowed {
			t.Fatalf("verifyBearer(%q) expected %v", c.header, c.allowed)
		}
	}
}
