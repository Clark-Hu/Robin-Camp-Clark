-- Initial schema for Movies API service.

-- Extensions -----------------------------------------------------------------
CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE EXTENSION IF NOT EXISTS btree_gin;

-- Helper function to keep updated_at columns in sync --------------------------
CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Movies ----------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS movies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title TEXT NOT NULL,
    release_date DATE NOT NULL,
    release_year INTEGER GENERATED ALWAYS AS (EXTRACT(YEAR FROM release_date)::INTEGER) STORED,
    genre TEXT NOT NULL,
    distributor TEXT,
    budget BIGINT CHECK (budget IS NULL OR budget >= 0),
    mpa_rating TEXT,
    box_office JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_movies_box_office_schema CHECK (
        box_office IS NULL OR (
            box_office ? 'revenue'
            AND (box_office -> 'revenue') ? 'worldwide'
            AND box_office ? 'currency'
            AND box_office ? 'source'
            AND box_office ? 'lastUpdated'
        )
    )
);

DROP TRIGGER IF EXISTS trg_movies_set_updated_at ON movies;
CREATE TRIGGER trg_movies_set_updated_at
BEFORE UPDATE ON movies
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE INDEX IF NOT EXISTS idx_movies_release_year ON movies (release_year);
CREATE INDEX IF NOT EXISTS idx_movies_genre ON movies (genre);
CREATE INDEX IF NOT EXISTS idx_movies_distributor_trgm ON movies USING GIN (distributor gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_movies_mpa_rating ON movies (mpa_rating);
CREATE INDEX IF NOT EXISTS idx_movies_budget ON movies (budget);
CREATE INDEX IF NOT EXISTS idx_movies_title_trgm ON movies USING GIN (title gin_trgm_ops);

-- Ratings ---------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS ratings (
    movie_id UUID NOT NULL REFERENCES movies(id) ON DELETE CASCADE,
    rater_id TEXT NOT NULL,
    rating NUMERIC(2,1) NOT NULL CHECK (rating IN (
        0.5, 1.0, 1.5, 2.0, 2.5, 3.0, 3.5, 4.0, 4.5, 5.0
    )),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (movie_id, rater_id)
);

DROP TRIGGER IF EXISTS trg_ratings_set_updated_at ON ratings;
CREATE TRIGGER trg_ratings_set_updated_at
BEFORE UPDATE ON ratings
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE INDEX IF NOT EXISTS idx_ratings_movie_id ON ratings (movie_id);
