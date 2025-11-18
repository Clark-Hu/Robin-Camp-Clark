# README **Movies API**

## **设计思路**

### **数据库选型与设计**

选择 **PostgreSQL** 的原因：

- 支持复杂查询、强一致事务和丰富数据类型
- 原生支持 JSONB、触发器
- 便于结构化存储电影和评分
- 未来扩展性强、生态成熟

表结构：

- **movies**：UUID 主键；字段含 title、release_date、release_year（生成列）、genre、distributor、budget、mpa_rating、box_office（JSONB）、created_at、updated_at 等，并设置 box_office 子字段约束。
- **ratings**：movie_id（UUID）、rater_id、rating、created_at、updated_at（触发器维护），复合主键 (movie_id, rater_id)；movie_id 外键 ON DELETE CASCADE；rating 有步进 CHECK 约束（0.5…5.0）。

> 注：main 分支仍保持 title 唯一以配合原始 e2e-test.sh；若出现同名需求，可使用 feature/non-unique-title 分支改动（见下文）。
> 

### **后端服务选型与设计**

- **Go + chi**：Go 为作业推荐语言；chi 提供轻量路由/中间件（RequestID、RealIP、Logger、Recoverer），结合标准 net/http 快速搭建。
- **分层结构**：各层职责清晰，从下到上依赖（domain → repository/store → http + boxoffice）：

    ```
    internal/
    ├── boxoffice       # BoxOffice 外部接口客户端与契约（对应监控逻辑）
    ├── config          # 环境变量配置加载/校验
    ├── domain          # 核心数据结构（Movie、Rating、BoxOffice 等）
    ├── http            # HTTP server/handlers（chi 路由、请求校验、输出格式）
    ├── repository      # 数据访问层（Movies/Ratings，基于 pgx）
    └── store           # 数据库连接池初始化、健康检查
    ```
1. **internal/domain**
    
    定义领域实体与值对象，供 repository/http 直接引用；不依赖其它业务层。
    
    - Movie：包含 ID、Title、ReleaseDate/Year、Genre、可选 Distributor/Budget/MpaRating/BoxOffice 与 CreatedAt/UpdatedAt。
    - BoxOffice、Revenue、Rating、RatingAggregate 等。
2. **internal/config**
    
    集中管理环境变量加载，保证 cmd/server/main.go 启动时不依赖硬编码。
    
    - Load()：读取 .env/环境变量并生成 Config（Port、AUTH_TOKEN、DB、BoxOffice 等），缺失必填项或非法值则报错。
3. **internal/store**
    
    封装数据库连接池及健康检查。
    
    - New(config.Config)：根据配置初始化 pgxpool。
    - HealthCheck(ctx)：执行 SELECT 1 检测可用性。
    - Pool()：向 repository 暴露连接池。
4. **internal/repository**
    
    数据访问层，只依赖 domain + pgx，对上提供明确接口。
    
    - repository.go：聚合 Repository{Movies, Ratings}。
    - movies.go：Create、GetByID、GetByTitle/FindByKeys、List（带分页/搜索）、UpdateMetadata（BoxOffice enrich）、游标编解码等。
    - ratings.go：Upsert（评分幂等）、Aggregate（平均值与计数）、Get。
5. **internal/http**
    
    HTTP 服务层，用 chi 注册路由、处理请求/响应。
    
    - server.go：Server 包含 config/store/repo/boxoffice/logger，registerRoutes 定义 /healthz、/movies CRUD、/movies/{}/ratings 等。
    - movies.go handlers：handleCreateMovie（校验 JSON → repo → BoxOffice enrich）、handleListMovie/handleGetMovie、handleSubmitRating、handleGetRating、辅助函数（解析、响应、roundToOneDecimal 等）。
6. **internal/boxoffice**
    
    外部 BoxOffice API 客户端与契约监控，隔离外部交互，便于 mock。
    
    - client.go：NewHTTPClient（baseURL/API key/timeout），Fetch 发送请求并解析 JSON，返回 Result。

### **测试体系**

- **常规测试**（go test ./...）
    - internal/config/config_test.go：配置加载/校验。
    - internal/repository/repository_test.go：嵌入式 Postgres 上的 CRUD、分页、评分 Upsert/Aggregate、并发 upsert、遍历迁移等。
    - internal/http/movies_handler_test.go：httptest 覆盖鉴权、参数校验、404/422 等。
    - internal/http/movies_filters_test.go：buildMovieFilters、verifyBearer。
    - internal/http/movies_test.go：辅助函数（如 roundToOneDecimal、allowedRatings）。
    - internal/boxoffice/client_contract_test.go：BoxOffice 客户端冒烟测试（供监控脚本使用）。
- **Fuzz 测试**（需 go test -fuzz）
    - internal/http/movies_fuzz_test.go → FuzzBuildMovieFilters。
    - internal/boxoffice/client_fuzz_test.go → FuzzConvertToResult。
- **Benchmark 测试**（需 go test -bench）
    - internal/repository/repository_test.go → BenchmarkMoviesRepositoryCreate、BenchmarkRatingsRepositoryUpsert。
    - internal/http/movies_benchmark_test.go → BenchmarkHandleSubmitRating。
- **E2E**（脚本）
    - e2e-test.sh：CRUD、评分、权限、错误处理、分页等场景，由 make test-e2e / GitHub Actions 触发。
- **BoxOffice 监控脚本**
    - scripts/monitor-boxoffice.sh：对照 boxoffice.openapi.yml 做探活与字段漂移检测（可配置 cron 定期运行）。

### **CI 集成**

- .github/workflows/ci.yml：包含 build-test 与 e2e 两个 job。
    - build-test：代码 checkout → 生成 .env → 注入 secrets → go fmt/go vet/go test ./...。
    - e2e：启动 docker compose、make test-e2e，失败收集日志。
    - 需要在 GitHub Repo 的 Secrets 中配置 AUTH_TOKEN、BOXOFFICE_API_KEY、DB_URL。
- 若因网络无法拉取镜像，可在 /etc/systemd/system/docker.service.d/http-proxy.conf 配置 Docker daemon 代理：
   ``` 
    [Service]
    Environment="HTTP_PROXY=http://ip:port"
    Environment="HTTPS_PROXY=http://ip:port"
    Environment="NO_PROXY=localhost,127.0.0.1,::1,host.docker.internal"`
    ```
- 当前 docker-compose.yml 仅用于开发环境，数据库数据保存在容器 /var/lib/postgresql/data。生产环境必须将数据与应用分离，严禁执行 make docker-down 等清理数据的命令。

### **项目可优化点**

1. API 资源标识统一使用 ID（feature 分支已在 /movies/{id} 实现），彻底消除同名歧义。
2. BoxOffice 服务监控：定期运行 scripts/monitor-boxoffice.sh + cron，检测上游可用性及字段漂移并邮件告警。
3. 搜索/排序：后端支持排序参数（title/year/rating），前端只选择字段，保持分页稳定。
4. BoxOffice 缓存：对热门影片结果做缓存，减少重复请求。
5. 鉴权统一：可考虑把 X-Rater-Id 合并进 Bearer token，设置过期刷新策略。
6. 智能化录入：创建电影时自动通过 title 查询 boxoffice，建议发行日期以辅助用户输入。
7. 性能与可观测性：部署监控，添加指标、结构化日志，对热点查询加索引/Explain 调优。

## **分支 feature/non-unique-title 变更摘要**

- 数据库：db/migrations/0002_title_release_genre_unique.up.sql 新增 (title, release_date, genre) 复合唯一约束，旧迁移保留。
- 仓储层：新增 FindByKeys 返回 title + releaseDate + genre 的匹配列表；GetByTitle 仅在唯一匹配时成功，其他情况 ErrNotFound。
- HTTP 路由/handler：
    - Rating/查询改为 ID 路由 /movies/{id}、/movies/{id}/ratings、/movies/{id}/rating（server.go、movies.go）。
    - 新增 handleGetMovie 通过 ID 查询。
- E2E：记录 movie1_id 并在所有评分/权限/错误路径使用 ID；搜索改为 title+genre+year 组合；列表校验组合键；404 场景使用固定无效 UUID；保留 legacy 注释。

---

**附**：常见环境建议

- 若 Docker 无法拉镜像，在 Docker daemon 级别设置代理或替换基础镜像。
- make test-e2e 已在 Makefile 中使用 env -u 清空代理变量，避免本地健康检查被代理拦截。
- 所有测试在离线环境（docker compose / embedded Postgres）运行，不影响生产数据；CI 只在 GitHub 临时 runner 上执行。