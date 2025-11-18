package repository

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Robin-Camp/Robin-Camp/internal/domain"
)

// MoviesRepository provides persistence helpers for movie entities.
type MoviesRepository struct {
	pool *pgxpool.Pool
}

const movieColumns = `
    id,
    title,
    release_date,
    release_year,
    genre,
    distributor,
    budget,
    mpa_rating,
    box_office,
    created_at,
    updated_at
`

// MovieCreateParams bundles the fields required to create a movie.
type MovieCreateParams struct {
	Title       string
	ReleaseDate time.Time
	Genre       string
	Distributor *string
	Budget      *int64
	MpaRating   *string
	BoxOffice   *domain.BoxOffice
}

// MovieListFilters encapsulates search and pagination options.
type MovieListFilters struct {
	Query       *string
	Year        *int
	Genre       *string
	Distributor *string
	BudgetLTE   *int64
	MpaRating   *string
	Limit       int
	Cursor      *MovieCursor
}

// MovieCursor allows stable pagination by created_at/id.
type MovieCursor struct {
	CreatedAt time.Time `json:"createdAt"`
	ID        string    `json:"id"`
}

// MovieListResult returns the paginated payload.
type MovieListResult struct {
	Items      []domain.Movie
	NextCursor *string
}

// Create inserts a new movie row and returns the stored entity.
func (r *MoviesRepository) Create(ctx context.Context, params MovieCreateParams) (domain.Movie, error) {
	boxOfficeJSON, err := marshalBoxOffice(params.BoxOffice)
	if err != nil {
		return domain.Movie{}, err
	}

	query := fmt.Sprintf(`
        INSERT INTO movies (title, release_date, genre, distributor, budget, mpa_rating, box_office)
        VALUES ($1,$2,$3,$4,$5,$6,$7)
        RETURNING %s
    `, movieColumns)

	row := r.pool.QueryRow(ctx, query, params.Title, params.ReleaseDate, params.Genre, params.Distributor, params.Budget, params.MpaRating, boxOfficeJSON)
	return scanMovie(row)
}

// FindByKeys fetches movies matching title with optional releaseDate/genre to disambiguate.
func (r *MoviesRepository) FindByKeys(ctx context.Context, title string, releaseDate *time.Time, genre *string) ([]domain.Movie, error) {
	where := []string{"title = $1"}
	args := []interface{}{title}
	if releaseDate != nil {
		where = append(where, fmt.Sprintf("release_date = $%d", len(args)+1))
		args = append(args, *releaseDate)
	}
	if genre != nil {
		where = append(where, fmt.Sprintf("genre = $%d", len(args)+1))
		args = append(args, *genre)
	}

	query := fmt.Sprintf(`SELECT %s FROM movies WHERE %s ORDER BY created_at DESC`, movieColumns, strings.Join(where, " AND "))
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []domain.Movie
	for rows.Next() {
		movie, err := scanMovie(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, movie)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

// GetByID fetches a movie by its identifier.
func (r *MoviesRepository) GetByID(ctx context.Context, id string) (domain.Movie, error) {
	query := fmt.Sprintf(`SELECT %s FROM movies WHERE id = $1`, movieColumns)
	row := r.pool.QueryRow(ctx, query, id)
	movie, err := scanMovie(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return domain.Movie{}, ErrNotFound
		}
		return domain.Movie{}, err
	}
	return movie, nil
}

// UpdateMetadata allows updating optional distributor/budget/mpaRating fields alongside box office payload.
func (r *MoviesRepository) UpdateMetadata(ctx context.Context, id string, distributor *string, budget *int64, mpaRating *string, boxOffice *domain.BoxOffice) (domain.Movie, error) {
	boxOfficeJSON, err := marshalBoxOffice(boxOffice)
	if err != nil {
		return domain.Movie{}, err
	}

	query := fmt.Sprintf(`
        UPDATE movies
        SET distributor = COALESCE($2, distributor),
            budget = COALESCE($3, budget),
            mpa_rating = COALESCE($4, mpa_rating),
            box_office = $5,
            updated_at = now()
        WHERE id = $1
        RETURNING %s
    `, movieColumns)

	row := r.pool.QueryRow(ctx, query, id, distributor, budget, mpaRating, boxOfficeJSON)
	movie, err := scanMovie(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return domain.Movie{}, ErrNotFound
		}
		return domain.Movie{}, err
	}
	return movie, nil
}

// List returns movies that match the provided filters.
func (r *MoviesRepository) List(ctx context.Context, filters MovieListFilters) (MovieListResult, error) {
	if filters.Limit <= 0 {
		filters.Limit = 20
	} else if filters.Limit > 100 {
		filters.Limit = 100
	}

	where := make([]string, 0)
	args := make([]interface{}, 0)
	arg := func(value interface{}) string {
		args = append(args, value)
		return fmt.Sprintf("$%d", len(args))
	}

	if filters.Query != nil && strings.TrimSpace(*filters.Query) != "" {
		q := "%" + strings.TrimSpace(*filters.Query) + "%"
		p1 := arg(q)
		p2 := arg(q)
		where = append(where, fmt.Sprintf("(title ILIKE %s OR distributor ILIKE %s)", p1, p2))
	}
	if filters.Year != nil {
		where = append(where, fmt.Sprintf("release_year = %s", arg(*filters.Year)))
	}
	if filters.Genre != nil && strings.TrimSpace(*filters.Genre) != "" {
		where = append(where, fmt.Sprintf("genre ILIKE %s", arg(strings.TrimSpace(*filters.Genre))))
	}
	if filters.Distributor != nil && strings.TrimSpace(*filters.Distributor) != "" {
		where = append(where, fmt.Sprintf("distributor ILIKE %s", arg(strings.TrimSpace(*filters.Distributor))))
	}
	if filters.BudgetLTE != nil {
		where = append(where, fmt.Sprintf("budget <= %s", arg(*filters.BudgetLTE)))
	}
	if filters.MpaRating != nil && strings.TrimSpace(*filters.MpaRating) != "" {
		where = append(where, fmt.Sprintf("mpa_rating ILIKE %s", arg(strings.TrimSpace(*filters.MpaRating))))
	}
	if filters.Cursor != nil {
		cursorCreated := arg(filters.Cursor.CreatedAt)
		cursorID := arg(filters.Cursor.ID)
		where = append(where, fmt.Sprintf("(created_at, id) < (%s, %s)", cursorCreated, cursorID))
	}

	queryBuilder := strings.Builder{}
	queryBuilder.WriteString("SELECT ")
	queryBuilder.WriteString(movieColumns)
	queryBuilder.WriteString(" FROM movies")

	if len(where) > 0 {
		queryBuilder.WriteString(" WHERE ")
		queryBuilder.WriteString(strings.Join(where, " AND "))
	}

	queryBuilder.WriteString(" ORDER BY created_at DESC, id DESC")
	queryBuilder.WriteString(fmt.Sprintf(" LIMIT %d", filters.Limit))

	rows, err := r.pool.Query(ctx, queryBuilder.String(), args...)
	if err != nil {
		return MovieListResult{}, err
	}
	defer rows.Close()

	items := make([]domain.Movie, 0)
	for rows.Next() {
		movie, err := scanMovie(rows)
		if err != nil {
			return MovieListResult{}, err
		}
		items = append(items, movie)
	}
	if err := rows.Err(); err != nil {
		return MovieListResult{}, err
	}

	var nextCursor *string
	if len(items) == filters.Limit {
		last := items[len(items)-1]
		cursor := MovieCursor{CreatedAt: last.CreatedAt, ID: last.ID}
		token, err := encodeCursor(cursor)
		if err != nil {
			return MovieListResult{}, err
		}
		nextCursor = &token
	}

	return MovieListResult{Items: items, NextCursor: nextCursor}, nil
}

func scanMovie(row pgx.Row) (domain.Movie, error) {
	var (
		movie         domain.Movie
		releaseDate   time.Time
		releaseYear   int
		distributor   *string
		budget        *int64
		mpaRating     *string
		boxOfficeJSON []byte
		createdAt     time.Time
		updatedAt     time.Time
	)

	err := row.Scan(
		&movie.ID,
		&movie.Title,
		&releaseDate,
		&releaseYear,
		&movie.Genre,
		&distributor,
		&budget,
		&mpaRating,
		&boxOfficeJSON,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return domain.Movie{}, err
	}

	movie.ReleaseDate = releaseDate
	movie.ReleaseYear = releaseYear
	movie.Distributor = distributor
	movie.Budget = budget
	movie.MpaRating = mpaRating
	movie.CreatedAt = createdAt
	movie.UpdatedAt = updatedAt

	if len(boxOfficeJSON) > 0 {
		var box domain.BoxOffice
		if err := json.Unmarshal(boxOfficeJSON, &box); err != nil {
			return domain.Movie{}, err
		}
		movie.BoxOffice = &box
	}

	return movie, nil
}

func marshalBoxOffice(boxOffice *domain.BoxOffice) ([]byte, error) {
	if boxOffice == nil {
		return nil, nil
	}
	return json.Marshal(boxOffice)
}

func encodeCursor(c MovieCursor) (string, error) {
	payload, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(payload), nil
}

// DecodeCursor parses a cursor token into a MovieCursor.
func DecodeCursor(token string) (*MovieCursor, error) {
	if token == "" {
		return nil, nil
	}
	data, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return nil, fmt.Errorf("invalid cursor: %w", err)
	}
	var cursor MovieCursor
	if err := json.Unmarshal(data, &cursor); err != nil {
		return nil, fmt.Errorf("invalid cursor payload: %w", err)
	}
	return &cursor, nil
}

// GetByTitle fetches the first movie matching a title. Ambiguous results return ErrNotFound.
func (r *MoviesRepository) GetByTitle(ctx context.Context, title string) (domain.Movie, error) {
	movies, err := r.FindByKeys(ctx, title, nil, nil)
	if err != nil {
		return domain.Movie{}, err
	}
	if len(movies) != 1 {
		return domain.Movie{}, ErrNotFound
	}
	return movies[0], nil
}
