package httpserver

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/Robin-Camp/Robin-Camp/internal/boxoffice"
	"github.com/Robin-Camp/Robin-Camp/internal/config"
	"github.com/Robin-Camp/Robin-Camp/internal/repository"
	"github.com/Robin-Camp/Robin-Camp/internal/store"
)

// Server wires HTTP routing, middleware, and handlers.
type Server struct {
	cfg       config.Config
	store     *store.Store
	repo      *repository.Repository
	boxOffice boxoffice.Client
	logger    *log.Logger
	router    chi.Router
	httpSrv   *http.Server
}

// New constructs the HTTP server with base middleware and routes.
func New(cfg config.Config, st *store.Store, repo *repository.Repository, boxClient boxoffice.Client, logger *log.Logger) *Server {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	if logger == nil {
		logger = log.Default()
	}

	s := &Server{
		cfg:       cfg,
		store:     st,
		repo:      repo,
		boxOffice: boxClient,
		logger:    logger,
		router:    r,
	}
	s.registerRoutes()
	return s
}

func (s *Server) registerRoutes() {
	s.router.Get("/healthz", s.handleHealthz)
	s.router.Route("/movies", func(r chi.Router) {
		r.Get("/", s.handleListMovies)
		r.Post("/", s.handleCreateMovie)
		r.Route("/{title}", func(r chi.Router) {
			r.Post("/ratings", s.handleSubmitRating)
			r.Get("/rating", s.handleGetRating)
		})
	})
}

// Start boots the HTTP server asynchronously.
func (s *Server) Start(ctx context.Context) error {
	s.httpSrv = &http.Server{
		Addr:         ":" + s.cfg.Port,
		Handler:      s.router,
		ReadTimeout:  time.Duration(s.cfg.ReadTimeoutSecs) * time.Second,
		WriteTimeout: time.Duration(s.cfg.WriteTimeoutSecs) * time.Second,
		IdleTimeout:  time.Duration(s.cfg.IdleTimeoutSecs) * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		if err := s.httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.httpSrv.Shutdown(shutdownCtx)
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}

// Shutdown gracefully stops the HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpSrv == nil {
		return nil
	}
	return s.httpSrv.Shutdown(ctx)
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := s.store.HealthCheck(ctx); err != nil {
		http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}
