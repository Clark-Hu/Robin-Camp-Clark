package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Clark-Hu/Robin-Camp-Clark/internal/boxoffice"
	"github.com/Clark-Hu/Robin-Camp-Clark/internal/domain"
	"github.com/Clark-Hu/Robin-Camp-Clark/internal/repository"
)

const maxRequestBody = 1 << 20 // 1 MiB

var allowedRatings = map[float32]struct{}{
	0.5: {}, 1.0: {}, 1.5: {}, 2.0: {}, 2.5: {},
	3.0: {}, 3.5: {}, 4.0: {}, 4.5: {}, 5.0: {},
}

type errorResponse struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Details interface{} `json:"details,omitempty"`
}

type movieCreateRequest struct {
	Title       string  `json:"title"`
	Genre       string  `json:"genre"`
	ReleaseDate string  `json:"releaseDate"`
	Distributor *string `json:"distributor"`
	Budget      *int64  `json:"budget"`
	MpaRating   *string `json:"mpaRating"`
}

type movieListResponse struct {
	Items      []movieResponse `json:"items"`
	NextCursor *string         `json:"nextCursor,omitempty"`
}

type movieResponse struct {
	ID          string             `json:"id"`
	Title       string             `json:"title"`
	ReleaseDate string             `json:"releaseDate"`
	Genre       string             `json:"genre"`
	Distributor *string            `json:"distributor,omitempty"`
	Budget      *int64             `json:"budget,omitempty"`
	MpaRating   *string            `json:"mpaRating,omitempty"`
	BoxOffice   *boxOfficeResponse `json:"boxOffice"`
}

type boxOfficeResponse struct {
	Revenue     revenueResponse `json:"revenue"`
	Currency    string          `json:"currency"`
	Source      string          `json:"source"`
	LastUpdated time.Time       `json:"lastUpdated"`
}

type revenueResponse struct {
	Worldwide        int64  `json:"worldwide"`
	OpeningWeekendUS *int64 `json:"openingWeekendUSA,omitempty"`
}

type ratingRequest struct {
	Rating float32 `json:"rating"`
}

type ratingResponse struct {
	MovieTitle string  `json:"movieTitle"`
	RaterID    string  `json:"raterId"`
	Rating     float32 `json:"rating"`
}

type ratingAggregateResponse struct {
	Average float32 `json:"average"`
	Count   int64   `json:"count"`
}

func (s *Server) handleListMovies(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	filters, err := buildMovieFilters(query)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	result, err := s.repo.Movies.List(r.Context(), filters)
	if err != nil {
		s.logger.Printf("list movies error: %v", err)
		s.respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list movies")
		return
	}

	items := make([]movieResponse, 0, len(result.Items))
	for _, movie := range result.Items {
		items = append(items, toMovieResponse(movie))
	}

	resp := movieListResponse{
		Items: items,
	}
	if result.NextCursor != nil {
		resp.NextCursor = result.NextCursor
	}
	s.respondJSON(w, http.StatusOK, resp)
}

func buildMovieFilters(query url.Values) (repository.MovieListFilters, error) {
	var filters repository.MovieListFilters

	if q := strings.TrimSpace(query.Get("q")); q != "" {
		filters.Query = &q
	}
	if val := strings.TrimSpace(query.Get("year")); val != "" {
		year, err := strconv.Atoi(val)
		if err != nil {
			return filters, fmt.Errorf("invalid year value")
		}
		filters.Year = &year
	}
	if val := strings.TrimSpace(query.Get("genre")); val != "" {
		filters.Genre = &val
	}
	if val := strings.TrimSpace(query.Get("distributor")); val != "" {
		filters.Distributor = &val
	}
	if val := strings.TrimSpace(query.Get("budget")); val != "" {
		budget, err := strconv.ParseInt(val, 10, 64)
		if err != nil || budget < 0 {
			return filters, fmt.Errorf("invalid budget value")
		}
		filters.BudgetLTE = &budget
	}
	if val := strings.TrimSpace(query.Get("mpaRating")); val != "" {
		filters.MpaRating = &val
	}
	if val := strings.TrimSpace(query.Get("limit")); val != "" {
		limit, err := strconv.Atoi(val)
		if err != nil {
			return filters, fmt.Errorf("invalid limit value")
		}
		filters.Limit = limit
	}
	if val := strings.TrimSpace(query.Get("cursor")); val != "" {
		cursor, err := repository.DecodeCursor(val)
		if err != nil {
			return filters, fmt.Errorf("invalid cursor")
		}
		filters.Cursor = cursor
	}
	return filters, nil
}

func (s *Server) handleCreateMovie(w http.ResponseWriter, r *http.Request) {
	if !s.verifyBearer(r.Header.Get("Authorization")) {
		s.respondError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Missing or invalid authentication information")
		return
	}

	var req movieCreateRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		s.respondDecodeError(w, err)
		return
	}

	releaseDate, err := time.Parse("2006-01-02", req.ReleaseDate)
	if err != nil {
		s.respondError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "releaseDate must follow YYYY-MM-DD format")
		return
	}
	if strings.TrimSpace(req.Title) == "" || strings.TrimSpace(req.Genre) == "" {
		s.respondError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "title and genre are required")
		return
	}
	if req.Budget != nil && *req.Budget < 0 {
		s.respondError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "budget must be non-negative")
		return
	}

	movie, err := s.repo.Movies.Create(r.Context(), repository.MovieCreateParams{
		Title:       strings.TrimSpace(req.Title),
		ReleaseDate: releaseDate,
		Genre:       strings.TrimSpace(req.Genre),
		Distributor: normalizeStringPtr(req.Distributor),
		Budget:      req.Budget,
		MpaRating:   normalizeStringPtr(req.MpaRating),
	})
	if err != nil {
		s.logger.Printf("create movie error: %v", err)
		s.respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create movie")
		return
	}

	enrichedMovie := s.enrichMovieWithBoxOffice(r.Context(), movie, req)

	location := fmt.Sprintf("/movies/%s", url.PathEscape(enrichedMovie.Title))
	w.Header().Set("Location", location)
	s.respondJSON(w, http.StatusCreated, toMovieResponse(enrichedMovie))
}

func (s *Server) enrichMovieWithBoxOffice(ctx context.Context, movie domain.Movie, req movieCreateRequest) domain.Movie {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(s.cfg.BoxOfficeTimeoutSecs)*time.Second)
	defer cancel()

	result, err := s.boxOffice.Fetch(ctx, movie.Title)
	if err != nil {
		if !errors.Is(err, boxoffice.ErrNotFound) {
			s.logger.Printf("boxoffice fetch failed for %s: %v", movie.Title, err)
		}
		return movie
	}

	distributor := firstNonNil(normalizeStringPtr(req.Distributor), result.Distributor)
	budget := firstNonNilInt(req.Budget, result.Budget)
	mpa := firstNonNil(normalizeStringPtr(req.MpaRating), result.MpaRating)

	updated, err := s.repo.Movies.UpdateMetadata(ctx, movie.ID, distributor, budget, mpa, result.BoxOffice)
	if err != nil {
		s.logger.Printf("update movie metadata failed: %v", err)
		return movie
	}
	return updated
}

func (s *Server) handleSubmitRating(w http.ResponseWriter, r *http.Request) {
	title, err := decodeTitleParam(r)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	raterID := strings.TrimSpace(r.Header.Get("X-Rater-Id"))
	if raterID == "" {
		s.respondError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Missing or invalid authentication information")
		return
	}

	movie, err := s.repo.Movies.GetByTitle(r.Context(), title)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			s.respondError(w, http.StatusNotFound, "NOT_FOUND", "Resource not found")
			return
		}
		s.logger.Printf("fetch movie for rating failed: %v", err)
		s.respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to process rating")
		return
	}

	var req ratingRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		s.respondDecodeError(w, err)
		return
	}
	if _, ok := allowedRatings[req.Rating]; !ok {
		s.respondError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "rating must be one of {0.5, 1.0, ..., 5.0}")
		return
	}

	rating, inserted, err := s.repo.Ratings.Upsert(r.Context(), repository.RatingUpsertParams{
		MovieID: movie.ID,
		RaterID: raterID,
		Value:   req.Rating,
	})
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			s.respondError(w, http.StatusNotFound, "NOT_FOUND", "Resource not found")
			return
		}
		s.logger.Printf("upsert rating error: %v", err)
		s.respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to process rating")
		return
	}

	status := http.StatusOK
	if inserted {
		status = http.StatusCreated
	}

	resp := ratingResponse{
		MovieTitle: movie.Title,
		RaterID:    rating.RaterID,
		Rating:     rating.Value,
	}
	s.respondJSON(w, status, resp)
}

func (s *Server) handleGetRating(w http.ResponseWriter, r *http.Request) {
	title, err := decodeTitleParam(r)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	movie, err := s.repo.Movies.GetByTitle(r.Context(), title)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			s.respondError(w, http.StatusNotFound, "NOT_FOUND", "Resource not found")
			return
		}
		s.logger.Printf("fetch movie for rating agg failed: %v", err)
		s.respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to fetch rating")
		return
	}

	agg, err := s.repo.Ratings.Aggregate(r.Context(), movie.ID)
	if err != nil {
		s.logger.Printf("aggregate rating error: %v", err)
		s.respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to fetch rating")
		return
	}

	resp := ratingAggregateResponse{
		Average: roundToOneDecimal(agg.Average),
		Count:   agg.Count,
	}
	s.respondJSON(w, http.StatusOK, resp)
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst interface{}) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	return nil
}

func (s *Server) respondJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if payload != nil {
		if err := json.NewEncoder(w).Encode(payload); err != nil {
			s.logger.Printf("failed to encode response: %v", err)
		}
	}
}

func (s *Server) respondError(w http.ResponseWriter, status int, code, message string) {
	s.respondJSON(w, status, errorResponse{
		Code:    code,
		Message: message,
	})
}

func (s *Server) respondDecodeError(w http.ResponseWriter, err error) {
	var syntaxError *json.SyntaxError
	var typeError *json.UnmarshalTypeError
	switch {
	case errors.As(err, &syntaxError):
		s.respondError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Malformed JSON payload")
	case errors.As(err, &typeError):
		s.respondError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", fmt.Sprintf("Invalid value for field %s", typeError.Field))
	case errors.Is(err, io.EOF):
		s.respondError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Request body cannot be empty")
	default:
		s.respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Unable to parse request body")
	}
}

func toMovieResponse(movie domain.Movie) movieResponse {
	resp := movieResponse{
		ID:          movie.ID,
		Title:       movie.Title,
		ReleaseDate: movie.ReleaseDate.Format("2006-01-02"),
		Genre:       movie.Genre,
		Distributor: movie.Distributor,
		Budget:      movie.Budget,
		MpaRating:   movie.MpaRating,
	}
	if movie.BoxOffice != nil {
		resp.BoxOffice = &boxOfficeResponse{
			Revenue: revenueResponse{
				Worldwide:        movie.BoxOffice.Revenue.Worldwide,
				OpeningWeekendUS: movie.BoxOffice.Revenue.OpeningWeekendUS,
			},
			Currency:    movie.BoxOffice.Currency,
			Source:      movie.BoxOffice.Source,
			LastUpdated: movie.BoxOffice.LastUpdated,
		}
	}
	return resp
}

func normalizeStringPtr(ptr *string) *string {
	if ptr == nil {
		return nil
	}
	val := strings.TrimSpace(*ptr)
	if val == "" {
		return nil
	}
	return &val
}

func firstNonNil[T any](primary, fallback *T) *T {
	if primary != nil {
		return primary
	}
	return fallback
}

func firstNonNilInt(primary, fallback *int64) *int64 {
	if primary != nil {
		return primary
	}
	return fallback
}

func decodeTitleParam(r *http.Request) (string, error) {
	raw := chi.URLParam(r, "title")
	if raw == "" {
		return "", fmt.Errorf("missing title parameter")
	}
	title, err := url.PathUnescape(raw)
	if err != nil {
		return "", fmt.Errorf("invalid title parameter")
	}
	return title, nil
}

func (s *Server) verifyBearer(header string) bool {
	if header == "" {
		return false
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	return token == s.cfg.AuthToken
}

func roundToOneDecimal(value float32) float32 {
	return float32(math.Round(float64(value)*10) / 10.0)
}
