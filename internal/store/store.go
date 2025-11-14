package store

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Options controls connection-pool behaviour.
type Options struct {
	MaxConns               int32
	MinConns               int32
	MaxConnIdleTime        time.Duration
	MaxConnLifetime        time.Duration
	ConnTimeout            time.Duration
	StatementCacheCapacity int
	Logger                 *log.Logger
}

// Store hides direct access to the underlying connection pool so higher layers
// can focus on business logic.
type Store struct {
	pool   *pgxpool.Pool
	logger *log.Logger
	opts   Options
}

// New initializes a connection pool and validates connectivity with Ping.
func New(ctx context.Context, dbURL string, opts Options) (*Store, error) {
	logger := opts.Logger
	if logger == nil {
		logger = log.Default()
	}
	logger.Printf("store: initializing connection pool (max=%d, min=%d, idle=%s, life=%s, stmt_cache=%d)",
		opts.MaxConns, opts.MinConns, opts.MaxConnIdleTime, opts.MaxConnLifetime, opts.StatementCacheCapacity)

	cfg, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		return nil, fmt.Errorf("parse db url: %w", err)
	}

	if opts.MaxConns > 0 {
		cfg.MaxConns = opts.MaxConns
	}
	if opts.MinConns > 0 {
		cfg.MinConns = opts.MinConns
	}
	if opts.MaxConnIdleTime > 0 {
		cfg.MaxConnIdleTime = opts.MaxConnIdleTime
	}
	if opts.MaxConnLifetime > 0 {
		cfg.MaxConnLifetime = opts.MaxConnLifetime
	}
	if opts.StatementCacheCapacity >= 0 {
		cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeCacheStatement
		cfg.ConnConfig.StatementCacheCapacity = opts.StatementCacheCapacity
	}

	connCtx := ctx
	if opts.ConnTimeout > 0 {
		var cancel context.CancelFunc
		connCtx, cancel = context.WithTimeout(ctx, opts.ConnTimeout)
		defer cancel()
	}

	pool, err := pgxpool.NewWithConfig(connCtx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}

	if err := pool.Ping(connCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	logger.Println("store: database connection established")

	return &Store{pool: pool, logger: logger, opts: opts}, nil
}

// Close releases database resources.
func (s *Store) Close() {
	if s == nil || s.pool == nil {
		return
	}
	s.logger.Println("store: closing connection pool")
	s.pool.Close()
}

// HealthCheck verifies the database is reachable.
func (s *Store) HealthCheck(ctx context.Context) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("store not initialized")
	}
	checkCtx := ctx
	if s.opts.ConnTimeout > 0 {
		var cancel context.CancelFunc
		checkCtx, cancel = context.WithTimeout(ctx, s.opts.ConnTimeout)
		defer cancel()
	}
	if err := s.pool.Ping(checkCtx); err != nil {
		return err
	}
	return nil
}

// Pool exposes the underlying pgx pool for repositories.
func (s *Store) Pool() *pgxpool.Pool {
	return s.pool
}

// Stats exposes pgxpool statistics for observability.
func (s *Store) Stats() *pgxpool.Stat {
	if s == nil || s.pool == nil {
		return nil
	}
	return s.pool.Stat()
}
