package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Robin-Camp/Robin-Camp/internal/boxoffice"
	"github.com/Robin-Camp/Robin-Camp/internal/config"
	httpserver "github.com/Robin-Camp/Robin-Camp/internal/http"
	"github.com/Robin-Camp/Robin-Camp/internal/repository"
	"github.com/Robin-Camp/Robin-Camp/internal/store"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	logger := log.New(os.Stdout, "[movies-api] ", log.LstdFlags|log.Lshortfile)

	dbCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	storeOpts := store.Options{
		MaxConns:               int32(cfg.DBMaxConns),
		MinConns:               int32(cfg.DBMinConns),
		MaxConnIdleTime:        time.Duration(cfg.DBMaxIdleSecs) * time.Second,
		MaxConnLifetime:        time.Duration(cfg.DBMaxLifeSecs) * time.Second,
		ConnTimeout:            time.Duration(cfg.DBConnTimeoutSecs) * time.Second,
		StatementCacheCapacity: cfg.DBStatementCache,
		Logger:                 logger,
	}

	st, err := store.New(dbCtx, cfg.DBURL, storeOpts)
	if err != nil {
		log.Fatalf("connect database: %v", err)
	}
	defer st.Close()

	boxClient, err := boxoffice.NewHTTPClient(cfg.BoxOfficeURL, cfg.BoxOfficeAPIKey, time.Duration(cfg.BoxOfficeTimeoutSecs)*time.Second, logger)
	if err != nil {
		log.Fatalf("init box office client: %v", err)
	}

	repo := repository.New(st)
	server := httpserver.New(cfg, st, repo, boxClient, logger)

	serverErrCh := make(chan error, 1)
	go func() {
		if err := server.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
			serverErrCh <- err
			return
		}
		serverErrCh <- nil
	}()

	select {
	case err := <-serverErrCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, context.Canceled) {
			log.Printf("server error: %v", err)
		}
	case <-ctx.Done():
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Printf("graceful shutdown error: %v", err)
	}
}
