# OpenSync 全量 Code Review 与修改建议

- 审查范围：Go 后端（backend + docker-entrypoint.go，约 1.7 万行）+ React/TS 前端（frontend/src + tests）+ Dockerfile / docker-compose / GitHub Actions
- 审查方式：逐文件精读（后端核心同步引擎、数据层、HTTP 层、通知/用户服务全部精读；前端关键 Hook、API、页面全部精读）+ 静态检查 + 编译/测试验证
- 审查日期：2026-08

---

## 0. 总体评价

**结论：这是一个工程质量明显高于平均水平的个人开源项目，核心同步引擎（扫描/复制/进度/持久化）并发模型严谨，未发现会造成崩溃或数据丢失的致命缺陷。**

已实测确认的工程基线（全部通过）：

| 检查项 | 结果 |
|---|---|
| `go build ./...` | ✅ 通过 |
| `go vet ./...` | ✅ 无告警 |
| `go test ./...`（全部包） | ✅ 全绿 |
| `npx tsc -b`（前端类型检查） | ✅ 通过 |
| `npx eslint .`（前端 Lint） | ✅ 0 错误 |
| `gofmt -l` | ⚠️ 10 个文件未格式化（下详） |

值得肯定的设计（建议保持，不要随意改动）：
- 同步引擎的信号量 + WaitGroup + context 取消链路完整，`copyTaskMonitor` 的 watch 生命周期（`stopped` 标志防"watch 无人消费导致 do 卡死"）处理得很小心；
- sqlite 并发写采用 `WAL + busy_timeout=5000 + MaxOpenConns=12`，批处理持久化 + 失败回填 Buffer，写竞争被控制得较好；
- 通知 URL 的 SSRF 防护（`ssrfSafeDialContext` + 解析后 IP 校验）、AList token 不落前端 DOM、cookie 更新即失效的会话签名（HMAC 含密码散列）都做对了；
- 前端轮询/SSE 的节流、去重、abort、页面可见性暂停、请求 ID 竞态防护非常规范。

以下按**高 / 中 / 低**三个优先级给出**修改建议**，每条都附具体文件、行号和可落地的改法。

---

## 1. 高优先级（建议尽快处理）

### H1. AList HTTP 客户端无并发连接上限、超时一刀切 300s
- 文件：`backend/internal/service/alist_client.go:52-59`
- 问题：每个 AList 引擎共享一个 `http.Client{Timeout: 300s}`，`Transport` 只配了 `MaxIdleConnsPerHost: 20`，**没有 `MaxConnsPerHost` 上限**，也没有分操作超时。当 `copy_concurrency` 调到高值、多个任务并发时，扫描 + 复制 + 轮询共用一个 Transport，活跃连接数无上限；任一请求挂起会钉住一个 worker goroutine 最多 5 分钟（复制轮询循环还会叠加 `fetchUndone` 的重负载）。极端情况下可能耗尽本地端口/触发 AList 侧连接数限制。
- 建议：
  1. `Transport` 增加 `MaxConnsPerHost`（如 32～64）；
  2. 按操作拆分超时：`list/mkdir/remove/info` 用 `ResponseHeaderTimeout`（如 20s）+ 整体超时（如 60s）；`fs/copy`、`fs/move` 提交类保留长超时（300s）；
  3. 给复制提交/轮询加每引擎的信号量，防止单引擎被打爆。

### H2. 大目录清单存在 10MB 硬上限，可能卡死/中断大目录同步
- 文件：`backend/internal/service/response_limit.go:8-19`、`alist_client.go:18`（`maxResponseBytes = 10MB`）
- 问题：`FileListApiContext` 把整个目录 JSON 读进内存并限制 10MB。对一个超大目录（数万～十几万文件，AList 返回带 `hash_info` 的完整清单）可能超过 10MB，同步扫描会直接硬失败（`task_scan.go:310-323` 再重试 2 次后把该目录标为扫描失败）。这对"同步整个照片库/影音库根目录"是真实风险。
- 建议：把上限提到 32MB 并支持配置项（如 `OPENSYNC_MAX_LIST_BYTES`）；更优做法是走 AList 的 `page` 分页参数流式拉完再合并，但改动大，可先做第一档。

### H3. 复制轮询每次全量拉取 AList 的"全部未完成任务"列表
- 文件：`backend/internal/service/copy_task_poller.go:199-224`（`fetchUndoneByType`）
- 问题：每个轮询周期（0.6s～3s）对每种 copy 类型调用 `/api/admin/task/copy|move/undone`，**把该 AList 实例上所有未完成任务（包括其他客户端/其他用户的）全部抓下来**再按 watch 匹配。高并发同步时这是持续的大响应，且把别人的任务也拉进内存。
- 建议：
  1. 只在 watch 数量 >=2 时才走"一次 undone 批量匹配"，watch 很少时直接逐 watch 调 `info`；
  2. 对 `undone` 返回做大小上限截断（防止超大响应），并丢弃不属于本任务 watch 集合的 ID；
  3. 当 `fetchUndone` 连续失败/超时时，本周期仍应退化为逐 watch `info`，避免整个轮询空转。

### H4.（安全）AList 客户端缺少与 notify 侧对等的 SSRF 约束
- 文件：`backend/internal/service/alist_client.go:46-66` vs `notify_service.go:41-65`
- 问题：通知客户端有 `ssrfSafeDialContext` 阻止内网/回环，但 AList 客户端没有。虽然 AList URL 只能由已登录管理员配置（非未授权可利用），但一旦管理员账号泄露或误配，工具可被当作内网探测/SSRF 跳板打到云元数据地址（169.254.169.254 等）。默认 `allow_internal_webhook=true` 也放开了 notify 到内网的路径。
- 建议：给 AList 客户端套同款 `Control` 校验（对齐 notify 的策略），并通过 `OPENSYNC_ALLOW_INTERNAL_ALIST` 之类的开关显式放行内网 AList（不少用户确实跑在局域网 NAS 上），默认值设为"允许"会破坏现有部署，故**建议默认跟随 allow_internal_webhook 或单列开关并文档化**。

---

## 2. 中优先级（建议近期处理）

### M1. panic 记录无堆栈，生产排障成本高
- 文件：`backend/internal/service/job_task.go:124-126`、`copy_task_poller.go`、`copy_item.go` 等处仅 `fmt.Sprintf("%v", r)`。
- 建议：在 `workerPanicMessage` 里附带 `debug.Stack()`（截断到 4KB）写入日志，否则"哪个 goroutine 因为什么 panic"基本不可查。

### M2. `GetTaskList` 的任务计数回填在并发出会"丢更新"
- 文件：`backend/internal/service/job_service.go:517-532`（`scheduleTaskNumUpdate`）
- 问题：`taskNumUpdateSlots` 容量 1，若上一次回填还在跑，新请求直接 `Skipping`，本次计数写不进去。虽然下次轮询会再算一遍，但瞬时展示的总数会不一致。
- 建议：改为"合并待办队列"（未完成先记到 buffer，完成后再扫一遍 buffer），或缩短该窗口。

### M3. 前端的 failed/other 大列表每次轮询都打后端
- 文件：`frontend/src/pages/Home/useRealtimeTaskItems.ts:108-166`
- 问题：非 running 的 tab（如"失败"有几十万条）固定 `POLL_INTERVAL_MS=3s` 轮询；虽然已做 20 条分页 + 服务端去重节流，但用户在失败页停留时仍会持续打 DB 聚合查询（`GetJobTaskCounts` 是 COUNT+SUM）。大任务页可能造成后端 sqlite 压力。
- 建议：进入"失败/其他/等待" tab 时把轮询频率降到 15s 或停止轮询、只在 SSE 收到新事件时刷新；running tab 保留 3s。

### M4. 配置热更新后 HTTP 客户端与旧连接不回收
- 文件：`backend/internal/service`（`UpdateClient`）、`config.go`（`UpdateSystemSettings`）
- 问题：`UpdateSystemSettings` 改 `copy/scan concurrency` 或 `maxRetries` 后，**运行中原有任务不会按新值执行**（`runtimeTaskLimits()` 是每次调用时读取，能生效，这点 OK）；但 `UpdateClient` 更新 AList 后，若 URL 未变且未带 token，内存中客户端不刷新（`alist_service.go:222-264` 逻辑是对的：URL 变但无 token 会报 WithoutToken）。这条主要是**确认行为**：文档要对用户说明"并发/重试改动只对之后启动的任务生效"。

### M5. `config.ini` 与 `data/` 目录权限
- 文件：`backend/internal/config/config.go:416`（chmod 0644）、`crypto.go:70`（0600）
- 现状：config.ini 0644 无敏感信息，可接受。建议顺手对 `data/` 目录统一 0750 并对 `secret.key`/`openSync.db` 保持 0600，避免同机其他低权限账号读到 DB（DB 已是 0600，OK）。

### M6. 历史任务删除用 `DELETE... IN (...)` 逐批 500 条，无 `VACUUM`/`incremental_vacuum` 规划
- 文件：`backend/internal/mapper/job_mapper.go:240-255`、`doDeleteExpired` 结尾只做 `wal_checkpoint(PASSIVE)`
- 问题：任务保留清理只删不整理，长期运行后 sqlite 文件只增不减（`job_task_item` 频繁 INSERT/DELETE）。单文件 DB 会持续膨胀。
- 建议：清理完成后按阈值执行 `PRAGMA incremental_vacuum` 或文档提醒定期 `VACUUM`。

### M7. 语言/时区：默认时区与 cron 行为
- 文件：`backend/internal/service/job_scheduler.go:35-56`、`task_retention_scheduler.go`
- 建议：`schedulerLocation` 用 `TZ` 环境变量，未设置回退 Asia/Shanghai，行为 OK；但要注意若宿主机 TZ 与"清理任务在每日 03:00"预期不一致，需要文档说明。可加一条启动日志"调度时区 = X"便于排查。

---

## 3. 低优先级 / 工程卫生（建议顺手做）

### L1. gofmt 未格式化（10 个文件，纯空格对齐）
`backend/internal/config/config.go`、`handler/{alist,job,system}_handler.go`、`middleware/auth.go`、`service/{job_client,job_service,job_task,user_service}.go`（+1 个测试文件）。运行：
```bash
cd backend && gofmt -w $(gofmt -l . | grep -v '/web/')
```
并把 `gofmt -l` 检查加进 CI。

### L2. CI 未做 vet / lint / Docker 构建门禁
- 文件：`.github/workflows/ci.yml`
- 现状：backend 只跑 `go test`，frontend 只跑 `npm test` + `npm run build`，**没有 `go vet`、ESLint、`docker build` 冒烟测试**。
- 建议：
  1. backend job 增加 `go vet ./...` 与 `gofmt -l` 检查（或引入 golangci-lint）；
  2. frontend job 增加 `npm run lint`；
  3. 增加一个 `docker build` 冒烟 job（`docker build .` 至少保证 Dockerfile 不坏），或在 PR 上跑 `docker buildx build --load` 校验。

### L3. Dockerfile 前端构建镜像版本与 CI 不一致
- 文件：`Dockerfile:4`（node:20-alpine）vs `.github/workflows/ci.yml`（Node 22）
- 建议：统一为相同大版本（建议 22），避免"CI 通过、Docker 构建失败"的版本漂移。

### L4. Dockerfile 无 HEALTHCHECK
- 建议：增加 `HEALTHCHECK CMD wget -qO- http://127.0.0.1:8023/ >/dev/null || exit 1`（scratch 基础镜像需先 COPY busybox 或 wget 静态二进制），配合 `restart: unless-stopped` 更稳。非必需，属锦上添花。

### L5. 前端 `usePathTree.onLoadData` 依赖抖动
- 文件：`frontend/src/pages/Home/usePathTree.ts:60-67`
- 问题：`onLoadData` 依赖 `loadedKeys`（数组状态），每次展开节点都会改变函数身份，`TreeSelect` 的 `loadData` 属性因此每次渲染都变化。
- 建议：把 `loadedKeys` 放到 `useRef<Set>` 并在回调里自行维护，函数用 `useCallback` 保持稳定（当前有 `includes` 守卫，风险低，属优化项）。

### L6. 通知脱敏存在理论边界
- 文件：`backend/internal/service/notify_service.go:171-177`（`isMaskedSecretValue`）
- 问题：判断"是否已脱敏"仅靠是否包含 `****`；若用户真实 secret 恰含 `****` 前缀会静默保留旧值。概率极低，但可改为"端到端标记"（如脱敏时写 `__MASKED__:{key}`，读取时精确比较）更严谨。

### L7. 登录/初始化端点的无限速面
- 现状：`ResetPassword`（PUT /svr/noAuth/login）已按"用户名+recovery"scope 限流；`Initialize`（POST /svr/noAuth/init）仅在未初始化时可调用，初始化后即拒绝 —— 均 OK。
- 建议（可选）：`GetInitStatus`/`Login` 等公开端点加个轻量全局 QPS 防刷（现在未初始化窗口很短，一般无需）。

### L8. 文档/注释一致性
- `README.md` 已非常完善；建议补两处：①"并发/超时/重试设置只对之后启动的任务生效"；②9xxx 之外 `OPENSYNC_ALLOW_INTERNAL_WEBHOOK` 的默认值（当前默认 true）及其安全含义。

---

## 4. 测试与覆盖建议（当前测试质量已很好）

- 全量 `go test ./...` 通过，service 包测试覆盖了并发队列、轮询取消、扫描冲突、重试、保留清理、FT5 关键字等关键路径，是亮点。
- 建议补充的高价值用例：
  1. **同文件名大小写/全角折叠的不确定性冲突**（`file_name_match.go`）的并发扫描去重；
  2. **任务超时（context deadline）** 时 `CopyItem` 处于 `retrying/waitForRemote` 状态的中断回归测试；
  3. **`GetTaskList` 计数回填在 `taskNumUpdateSlots` 满时丢更新**的行为断言（对应 M2）；
  4. 前端已有基于 `node --test` 的单元测试（`frontend/tests/*`），建议把 `useRealtimeTaskItems` 的节流/abort 竞态也纳入（当前主要测布局/工具函数）。

---

## 5. 结论

按优先级建议的执行顺序：
1. **H1 & H2 & H3**（并发/大目录/轮询负载）→ 直接关系到大目录、高并发同步的稳定性；
2. **H4**（SSRF 对称化）→ 安全加固，改动小；
3. **M1/M2/M3**（日志可观测、计数回填、前端轮询降频）→ 排障与资源；
4. L 系列 → CI 门禁、gofmt、版本统一等工程卫生，改动极小、收益立竿见影。

核心同步引擎的并发设计无需重写；以上均为**增量加固**，风险可控、可逐步合入。

---

## 6. 落地进度（2026-08 已实现）

本轮已直接在仓库中实现并验证的修复（编译/测试/静态检查全绿，见第 0 节基线）：

| 编号 | 落地内容 | 主要文件 |
|---|---|---|
| H1 | AList 客户端连接上限 `MaxConnsPerHost=32` + `ResponseHeaderTimeout=30s` | `backend/internal/service/alist_client.go` |
| H2 | 列表响应上限提至 32MB，支持 `OPENSYNC_MAX_LIST_BYTES` 覆盖（≥1MB）；测试改为临时降限 | `backend/internal/service/alist_client.go`、`response_limit_test.go` |
| H3 | 复制轮询仅当 watch>1 时才全量拉取 undone 列表；只保留本任务关注的 ID；加 `maxUndonePickups` 上限 | `backend/internal/service/copy_task_poller.go` |
| H4 | AList 客户端接入 SSRF 防护（与 notify 共用 `ssrfSafeDialContext`），新增 `allow_internal_alist` / `OPENSYNC_ALLOW_INTERNAL_ALIST`（默认 true 不破坏局域网部署）；配置读取改为"拨号时惰性求值" | `backend/internal/service/alist_client.go`、`notify_service.go`、`config/config.go` |
| M1 | panic 日志附带 `debug.Stack()`（截断 4KB） | `backend/internal/service/job_task.go` |
| M2 | 任务计数回填改为"合并待处理 map + 持续 drain"，并发生不再丢更新 | `backend/internal/service/job_service.go` |
| M3 | 前端非运行中 tab 轮询间隔从 3s 放宽到 15s | `frontend/src/pages/Home/useRealtimeTaskItems.ts` |
| L1 | 全部 Go 文件 gofmt 格式化（含此前 10 个未格式化文件）；CI 增加 gofmt 检查门禁 | 全量 backend + `.github/workflows/ci.yml` |
| L2 | CI 增加 `go vet` 与前端 `npm run lint` 门禁 | `.github/workflows/ci.yml` |
| L3 | Dockerfile 前端构建镜像 node:20-alpine → node:22-alpine（与 CI 对齐） | `Dockerfile` |
| L4 | 新增 `opensync healthcheck` 子命令 + Dockerfile `HEALTHCHECK`（不触碰数据卷，任意用户可跑） | `backend/cmd/server/main.go`、`Dockerfile` |
| L6 | `usePathTree` 的 `loadedKeys` 改为 ref 镜像，稳定 `onLoadData` 函数身份 | `frontend/src/pages/Home/usePathTree.ts` |

已实现项验证结果：`go build ./...` ✅、`go vet ./...` ✅、`go test ./...`（全部包）✅、`tsc -b` ✅、`eslint` ✅、前端 `npm test` 63/63 ✅、`gofmt -l` 干净 ✅。

未落地（建议后续单独处理）：M4（热更新语义文档化）、M5（sqlite 膨胀的 `incremental_vacuum`）、M6（调度时区日志）、L5（docker-compose 加固）、L7（公开端点 QPS）、L8（README 补充两项说明）。
