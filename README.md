# Movies API

## 设计思路

### 数据库选型与设计

选择**PostgreSQL**的原因：

    - 支持复杂查询，强一致事务，数据类型丰富;

    - 原生支持 JSONB/触发器;

    - 便于结构化存储电影、评分;

    - 未来扩展性强,成熟的数据库

表结构
  - movies：UUID 主键，基础字段（title、release_date、release_year 生成列、genre、distributor、budget、mpa_rating、box_office JSONB)，created_at, updated_at 触发器保持更新时间。
     设置box_office 子字段的约束
  - ratings：moive_id UUID, rater_id, rating, created_at, updated_at 触发器保持更新时间
     (movie_id, rater_id) 复合主键 + CHECK 约束限制评分步进（0.5…5.0），对movie_id 外键设置ON DELETE CASCADE 保持一致性。

注：考虑到现实存在同名电影，而e2e_test.sh中部分测试只包含title进行查询，为了避免改动原始的e2e_testh.sh，新开分支feature/non-unique-title处理同名电影情况， 唯一约束改为 (title, release_date, genre)，支持同名不同档期/类型共存。
   main 分支仍然是title 唯一，不允许插入同名电影

### 后端服务选型与设计

- **Go + chi**：轻量路由与中间件（RequestID、RealIP、Logger、Recoverer），快速定制，配合标准库 net/http 实现 handler。

- **存储层**：pgxpool 直连 Postgres，仓储层封装 Movies/Ratings CRUD、分页、游标、UPSERT（评分）。

- **配置**：internal/config 从环境读取，必填项（AUTH_TOKEN、DB_URL、BOXOFFICE_URL/API_KEY 等）做运行时校验。

- **BoxOffice 集成**：可配置的 HTTP 客户端，支持超时/错误处理，boxOffice 数据 JSONB 持久化。

  注意：

  . 如果因为网络原因无法拉取镜像， 在/etc/systemd/system/docker.service.d/http-proxy.conf 设置适当的代理

  [Service]
  Environment="HTTP_PROXY=http://ip:port"
  Environment="HTTPS_PROXY=http://ip:port"
  Environment="NO_PROXY=localhost,127.0.0.1,::1,host.docker.internal"

  . 当前docker-compose.yml仅仅为开发环境设计，数据库数据保存在docker的/var/lib/postgresql/data，生产环境必须将数据文件和应用分离，严禁执行make docker-down类似命令

### 项目可优化点

- API 资源标识统一用 ID（当前 feature 分支已在评分/查询路由上切换到 /movies/{id}），其他 handler/客户端也可迁移，彻底消除同名歧义。
- 搜索/排序：后端支持排序参数（title/year/rating），前端仅选择字段，保持分页稳定。
- 更细粒度的错误映射与幂等性（如电影创建遇唯一冲突返回 409 或幂等响应）。
- 性能与可观测性：添加指标、日志结构化，必要时对热点查询加索引/Explain 调优。
- CI 缓存/测试耗时优化（embedded Postgres 测试时间较长，可复用实例或切换 testcontainers）。

## 分支 feature/non-unique-title 变更摘要

- **数据库**：db/migrations/0002_title_release_genre_unique.up.sql 新增复合唯一约束 (title, release_date, genre)，旧迁移不改。

- **仓储层**：新增 FindByKeys(title, releaseDate, genre) 返回匹配列表；GetByID 仍按 UUID 取单条，GetByTitle 仅在唯一匹配时返回。

- HTTP 路由/handler

  ：

  - Rating/获取改为 ID 路由：/movies/{id}, /movies/{id}/ratings, /movies/{id}/rating（server.go, movies.go）。
  - 新增 handleGetMovie 基于 ID 返回单条。

- E2E 脚本

  ：

  - 创建时记录 movie1_id/movie2_id 及 title/releaseDate/genre。
  - 所有评分/权限/错误场景改用 /movies/{id}（含 401/404/422）。
  - 搜索改为 title+genre+year 组合，列表校验包含创建的组合键；旧 title-only 搜索保留注释说明。
  - 404 场景用固定无效 UUID。

- **监控/契约**：scripts/monitor-boxoffice.sh 加入对真实 BoxOffice 上游的探测（基于 .env），可邮件告警；配合 TestHTTPClientSmoke。可在 cron 中定期跑，防止上游字段/可用性变更不知情。

## 运行与测试

- 本地运行：make docker-up 或直接 go run ./cmd/server（需 .env 准备）。
- 单元/集成：go test ./...（部分测试使用 embedded Postgres，时间较长）。
- E2E：make test-e2e（依赖 docker-compose 启动服务）。
- BoxOffice 监控：ENV_FILE=.env ALERT_EMAIL=you@example.com ./scripts/monitor-boxoffice.sh（需 mail 命令可用）。
- 

# BoxOffice Service

一个面向电影票房/评分的后端服务，提供电影的 CRUD、评分与搜索等能力，同时具备对上游 BoxOffice API 的契约监控与本地 mock 能力。

------

## 系统设计概述

### 1. 数据库选型与设计

| 项目       | 方案                                         | 说明                                                         |
| ---------- | -------------------------------------------- | ------------------------------------------------------------ |
| 引擎       | PostgreSQL                                   | 支持复杂查询、事务与丰富的数据类型，便于未来扩展（JSONB、全文检索等）。 |
| 连接管理   | `github.com/jackc/pgx/v5`                    | 官方推荐驱动，性能稳定。                                     |
| 嵌入式测试 | `github.com/fergusstrange/embedded-postgres` | 让集成测试在本地、CI 都可使用真实 PG 实例，无需额外依赖。    |

**Schema 演进**

1. **`0001_\*.sql`（保留）**：初版电影表，以 `title` 作为唯一约束。
2. `0002_title_release_genre_unique.up.sql`（新迁移）
   - 唯一键改为 `(title, release_date, genre)`，解决同名不同版本的电影冲突。
   - 与 `feature/non-unique-title` 分支同步，引入组合键思维。
3. **主键**：新增 UUID `id`，所有外部交互改用 `id`，避免模糊匹配。
4. **仓储层**：`MoviesRepository` 增加 `FindByKeys(title string, releaseDate, genre *string)`，让 handler 按需要通过组合键 disambiguate。

### 2. 后端服务选型与设计

| 层级             | 技术/结构                                                    | 设计说明                                                     |
| ---------------- | ------------------------------------------------------------ | ------------------------------------------------------------ |
| HTTP 框架        | `chi v5`                                                     | 轻量、高性能，路由中间件丰富。                               |
| Handler 结构     | 按资源拆分（movies、ratings）                                | 便于分模块维护与测试。                                       |
| 路由策略（最新） | 以 **ID 驱动**：`/movies/{id}`、`/movies/{id}/ratings`、`/movies/{id}/rating` | 消除同名电影歧义；搭配 `decodeMovieIDParam` 与仓储 `GetByID`。 |
| BoxOffice 依赖   | `cmd/boxoffice-tool serve`（mock） + `cmd/boxoffice-tool check`（契约对比） | mock 支持本地开发；契约对比用于监控脚本/CI。                 |
| 监控脚本         | `scripts/monitor-boxoffice.sh`                               | 新增对 BoxOffice 上游的 JSON 字段比对，若与 `boxoffice.openapi.yml` 不符或服务不可用就告警。另有 cron job 周期执行。 |

### 3. 端到端（E2E）测试设计

- 脚本：`e2e-test.sh`
- 关键更新：
  - Stage 2 创建电影后记录 `movie*_id` 以及 `title/releaseDate/genre` 变量。
  - 列表校验确保出现同组合键的记录，防止同名影片造成误判。
  - 搜索改为组合过滤（title+genre+year），并校验结果。
  - 所有评分、权限、错误用例均改为 `/movies/{id}` 路径；404 使用固定无效 UUID。
  - 保留了旧 title-only 搜索的注释，方便回溯。

------

## BoxOffice 契约监控与 Mock

### 单一 CLI：`cmd/boxoffice-tool`

| 子命令  | 用途                                                         |
| ------- | ------------------------------------------------------------ |
| `serve` | 启动 mock BoxOffice 服务（默认 9099），读取 `mock-boxoffice.json` 响应 `/boxoffice?title=...`。 |
| `check` | 将某次请求返回的 JSON 与 `boxoffice.openapi.yml` 的 `BoxOfficeRecord` schema 做字段/类型对比，发现漂移即退出码 1。 |

### 监控脚本

`scripts/monitor-boxoffice.sh` 流程：

1. 运行健康探测。
2. 抓取样例 payload，调用 `go run ./cmd/boxoffice-tool check ...`。
3. 通过 cron job 周期执行；失败会触发 `send_alert`（可接邮件/Slack 等）。

------

## 当前分支：`feature/non-unique-title`

在该分支完成的关键修改：

1. **数据库层**
   - 新迁移 `db/migrations/0002_title_release_genre_unique.up.sql`：唯一约束改为 `(title, release_date, genre)`。
   - 旧迁移保持不变，方便历史回放。
2. **仓储层**
   - `MoviesRepository.FindByKeys` 新增，允许按 `title` + 可选 `releaseDate`、`genre` 查询列表。
   - handler 通过 `id` 或组合键消除歧义。
3. **HTTP 路由 & Handler**
   - 所有 rating/获取接口改为 ID 路径（`/movies/{id}` 等）。
   - 新增 `handleGetMovie`；内部通过 `decodeMovieIDParam` → `GetByID` 获取电影。
   - `internal/http/server.go` 与 `internal/http/movies.go` 对应更新。
4. **E2E 测试脚本**
   - 使用 `movie1_id` 等变量驱动全套流程。
   - 列表、搜索、评分、权限、错误场景均以 ID 触发。
   - 引入组合过滤、列表包含校验，确保同名场景覆盖。

------

## 设计思路回顾

1. **数据准确性优先**
   - 引入组合唯一键与 UUID 主键，确保“同名不同版本”也能精准定位。
   - 合同式监控保证外部依赖变更不会悄悄破坏接口。
2. **可测试性**
   - embedded Postgres + mock BoxOffice + e2e 测试，使 CI 能稳定复现真实场景。
   - `boxoffice-tool` 集中 mock & check 功能，降低维护成本。
3. **可观测性**
   - 监控脚本 + cron job，实现对上游接口健康与 schema 漂移的持续关注。

------

## 可优化方向（未来工作）

1. **更严格的契约校验**
   - 目前只比较字段 & 基本类型，可进一步使用 JSON Schema 校验（enum、format、required）。
   - 结合 Prometheus 指标或 Slack bot，提供结构化告警。
2. **搜索与索引优化**
   - 对 `(title, release_date, genre)` 组合建立索引，并评估全文检索（PG trigram 或外部引擎）。
   - 支持更丰富的过滤（导演、语言、地区等）。
3. **缓存与性能**
   - 热门电影/评分可放入 Redis 或内存缓存。
   - 针对 BoxOffice 上游增加熔断/重试机制，减少监控误报。
4. **API 版本化与文档**
   - 目前依赖 OpenAPI 文件，可考虑自动生成文档站点，或提供 SDK。
   - 对外暴露的 API 可引入 `/v1` namespace，方便未来破坏性修改。
5. **权限与多租户**
   - 现阶段权限测试仅覆盖基础场景，可扩展角色/租户模型，支持更精细化的 RBAC。
6. **CI/CD 集成**
   - 将 `boxoffice-tool check`、e2e 测试、数据库迁移检测纳入 PR/merge pipeline。
   - cron 结果写入监控面板，形成可视化趋势。

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


##  Make 
- 启动加入参数-d可以后台启动，若希望前台运行输出日志，去掉-d