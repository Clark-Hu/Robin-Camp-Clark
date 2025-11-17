# BoxOffice 集成自检/告警脚本说明

## 目的

BoxOffice 上游可能在未通知的情况下调整字段或短暂不可用。脚本 `scripts/monitor-boxoffice.sh` 提供一次性或定时的“契约与可用性”自检，直接以真实上游为主（可从 `.env` 读取环境）：
- 用本地 OpenAPI/解析逻辑跑单测（不依赖真实上游）。
- 必须提供真实 `BOXOFFICE_URL`/`BOXOFFICE_API_KEY`，脚本会探测真实上游可用性/契约兼容性。
- 失败即退出非零，可被 cron/监控系统捕获后报警；可选 `ALERT_EMAIL` + `mail` 命令发送告警。

## 原理

1. **契约自测**：`go test ./internal/boxoffice` 跑解析相关的单测/模糊测试，保证本地结构体与 OpenAPI 契约一致。
2. **真实上游探活**：从 `.env`（或 `ENV_FILE` 指定）加载 `BOXOFFICE_URL`/`BOXOFFICE_API_KEY`，用 `TestHTTPClientSmoke` 直接请求一次真实 BoxOffice API，验证可用性和响应结构。失败直接报错/告警。

## 使用方式

### 手动执行

```bash
# 读取 .env 中的 BOXOFFICE_URL / BOXOFFICE_API_KEY 并探测真实上游
./scripts/monitor-boxoffice.sh

# 显式指定 env 文件与告警邮箱（需本机有 mail 命令）
ENV_FILE=/path/to/.env ALERT_EMAIL=ops@example.com ./scripts/monitor-boxoffice.sh
```

日志会输出到 `monitor-logs/boxoffice_monitor_<timestamp>.log`。

### 定时任务示例（cron）

每 5 分钟运行一次，仅对本地契约+mock 自检，失败则报警（需配合主机的告警机制）：

```
*/5 * * * * cd /path/to/repo && ./scripts/monitor-boxoffice.sh || echo "BoxOffice monitor failed" >&2
```

如果希望顺带探测真实上游，提前在环境里配置 `BOXOFFICE_URL` / `BOXOFFICE_API_KEY`，并设置 `MOCK_ONLY=false`。

## 相关文件

- `scripts/monitor-boxoffice.sh`：自检脚本本体（加载 .env，探测真实上游，可选邮件告警）。
- `internal/boxoffice/client_contract_test.go`：供脚本调用的 HTTP 冒烟测试用例。

## 注意事项

- 脚本仅对契约/可用性做“冒烟”级检查，不替代功能性 E2E 测试。
- 若上游长期不可用或字段变更，脚本会非零退出，应结合监控/告警系统接收失败事件。
- 本地 mock 使用静态数据，如需覆盖新增字段，请更新 `mock-boxoffice.json` 并重跑脚本。
