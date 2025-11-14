# Movies API (Work-in-Progress)

> ⚠️ **Note**: This README is a working draft that summarizes the current scaffold. Please adapt the wording to your own style before submission to comply with the assignment rules.

## Overview

This repository hosts a Go implementation of the Movies API described in `openapi.yml`. The service persists movie metadata and ratings in PostgreSQL, enriches newly created movies by calling the Box Office mock API, and exposes endpoints that mirror the provided contract. Docker Compose orchestrates the application, database, and schema migrations so the stack can be bootstrapped with a single command.

## Local Development

1. Copy the sample environment file and edit the values:
   ```bash
   cp .env.example .env
   ```
2. Apply database migrations to your local Postgres (if you are not using Compose yet):
   ```bash
   migrate -path db/migrations -database "$DB_URL" up
   ```
3. Start the Go API with the helper script (it automatically loads `.env`):
   ```bash
   ./scripts/run-local.sh
   ```
4. Execute the end-to-end test suite once the server is reachable:
   ```bash
   ./e2e-test.sh
   ```

To use Docker Compose end-to-end:

```bash
docker compose up --build                # builds app, applies migrations, runs every container
./e2e-test.sh                            # run from host once the stack is healthy
docker compose down -v                   # stop and clean volumes
```

## Configuration Reference

All configuration must be supplied via environment variables (no hard-coded secrets). The table below lists every key the service consumes, along with suggested defaults:

| Variable | Description | Default |
| --- | --- | --- |
| `PORT` | HTTP port for the API | `8080` |
| `AUTH_TOKEN` | Static Bearer token required for write operations | _(no default, must set)_ |
| `DB_URL` | PostgreSQL connection string | `postgres://movies:moviespass@postgres:5432/movies?sslmode=disable` (Compose) |
| `BOXOFFICE_URL` | Base URL of the Apifox mock upstream | `https://apifoxmock.com/m1/7149601-6873494-default` |
| `BOXOFFICE_API_KEY` | `X-API-Key` for the mock upstream | _(no default, must set)_ |
| `BOXOFFICE_TIMEOUT_SECS` | Upstream HTTP timeout (seconds) | `5` |
| `SERVER_READ_TIMEOUT` | HTTP read timeout (seconds) | `15` |
| `SERVER_WRITE_TIMEOUT` | HTTP write timeout (seconds) | `15` |
| `SERVER_IDLE_TIMEOUT` | HTTP idle timeout (seconds) | `60` |
| `DB_MAX_CONNS` | Max pooled DB connections | `20` |
| `DB_MIN_CONNS` | Min pooled DB connections (pre-warmed) | `2` |
| `DB_MAX_CONN_IDLE_SECS` | Idle connection reap interval | `300` |
| `DB_MAX_CONN_LIFETIME_SECS` | Max connection lifetime | `3600` |
| `DB_CONN_TIMEOUT_SECS` | Timeout for establishing or pinging a DB connection | `10` |
| `DB_STATEMENT_CACHE_CAPACITY` | pgx statement cache capacity | `256` |

The `.env` / `.env.example` files demonstrate how these variables can be organized for local development. Docker Compose loads `.env` automatically and propagates each variable into the `app` service.

## Repository Layout

- `cmd/server` – application entrypoint that wires config, storage, repositories, and HTTP transport.
- `internal/config` – environment loading and validation logic.
- `internal/store` – pgx connection pool setup with observability hooks.
- `internal/repository` – persistence layer for movies and ratings (upsert logic, pagination helpers, aggregates).
- `internal/http` – Chi router plus shared middleware; business handlers will live here.
- `db/migrations` – SQL migrations (extensions, tables, constraints, indexes).
- `scripts/run-local.sh` – convenience script that loads `.env` and runs `go run ./cmd/server`.

## Architecture Overview

- **HTTP Layer (`internal/http`)** — Chi-based router, request validation, auth guards, and OpenAPI-compliant handlers (`GET/POST /movies`, `/movies/{title}/ratings`, `/movies/{title}/rating`, `/healthz`). Responses mirror the contract structures and reuse the domain models.
- **Box Office Client (`internal/boxoffice`)** — Thin HTTP client that calls the Apifox mock (`/boxoffice?title=...`) with timeout + logging. On success it returns distributor/budget/mpaRating/boxOffice payloads so movies can be enriched; on 404 it falls back to `boxOffice = null` without blocking creation.
- **Domain & Repository (`internal/domain`, `internal/repository`)** — Domain structs (`Movie`, `Rating`, `BoxOffice`) describe the canonical data shape. Repositories encapsulate SQL (create/list/update, cursor pagination, rating upsert/aggregate) on top of the shared pgx pool from `internal/store`.
- **Store (`internal/store`)** — Owns the pgx connection pool with env-driven max/min connections, idle/lifetime limits, connection timeouts, and statement cache capacity. Exposes a health check and pool stats so the HTTP layer can report readiness.

### Movie Creation Flow
1. Client hits `POST /movies` with Bearer token and required fields (`title`, `genre`, `releaseDate`).
2. Service writes the base record, then asynchronously (still in request path) calls the Box Office client. If upstream returns data, optional fields (`distributor`, `budget`, `mpaRating`) are filled only when the requester left them blank, and the `boxOffice` JSONB column is populated; otherwise box office remains `null`.
3. Response returns `201 Created`, the merged movie payload, and a `Location` header.

### Ratings Flow
- `POST /movies/{title}/ratings` requires header `X-Rater-Id`, validates rating ∈ {0.5, …, 5.0}, and runs an upsert. First-time submissions return `201`, updates return `200`.  
- `GET /movies/{title}/rating` checks movie existence, aggregates average/count (average rounded to one decimal), and returns `{average, count}` even when `count = 0`.

## Testing & Next Steps

- With the handlers implemented, local development can proceed to the verification stage. Launch the server (`./scripts/run-local.sh` or `docker compose up --build`) and run the provided `./e2e-test.sh` once the `/healthz` endpoint reports healthy.
- Add unit tests around repositories or handler helpers as needed. For regression coverage, the provided E2E script exercises authentication, pagination, box office enrichment fallbacks, and validation errors (422 responses).
- Keep iterating on this README to describe the final architecture, trade-offs, and future improvements before submission.
