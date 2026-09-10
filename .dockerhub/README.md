# OpenSync

OpenSync is an AList / OpenList automation tool for fnOS (FeiNiu NAS), general NAS, and Docker environments. It connects local folders, cloud drives, object storage, WebDAV, and other storage backends through AList / OpenList, and uses visual jobs to handle backup, mirror, archive, and migration workflows.

![Task Management](https://raw.githubusercontent.com/chenbin3625/OpenSync/main/docs/images/tasks-overview.png)

## Quick Start

```bash
docker run -d \
  --name opensync \
  --restart unless-stopped \
  -p 8023:8023 \
  -v opensync-data:/app/data \
  -e TZ=Asia/Shanghai \
  -e OPENSYNC_BIND=0.0.0.0 \
  -e OPENSYNC_PORT=8023 \
  -e GIN_MODE=release \
  chenbin3625/opensync:latest
```

Then open `http://<your-device-ip>:8023/` and follow the prompts to create an admin username and password.

On first start, a 24-digit recovery key is shown once — save it immediately. If you forget the password, set a new one on the web page with "username + recovery key" (the old key becomes invalid and a new one is generated). If the recovery key is also lost, run the CLI fallback:

```bash
docker exec -it opensync ./opensync reset-password --user admin
```

Docker Compose is the recommended deployment:

```bash
mkdir -p opensync
cd opensync
curl -O https://raw.githubusercontent.com/chenbin3625/OpenSync/main/docker-compose.yml
docker compose up -d
```

## Image Tags

| Tag | Description |
| --- | --- |
| `chenbin3625/opensync:latest` | Latest stable release, recommended by default |
| `chenbin3625/opensync:1.13.2` | Pinned full version, for reproducible deployments |
| `chenbin3625/opensync:1.13` | Minor-version tag, tracks the newest `1.13.x` patch release |
| `chenbin3625/opensync:1.10` | Older minor-version tag kept for existing deployments |

Images are published on every official GitHub release by the Docker Hub workflow, which pushes the full version, the minor version (`1.13`), and `latest` (prereleases do not update `latest`).

Supported platforms:

- `linux/amd64`
- `linux/arm64`
- `linux/arm/v7`

## Key Features

- Multiple sources and targets: sync one or more source directories to one or more target directories.
- Three sync modes: Add-only, Full sync, and Move — for incremental backup, target mirroring, and archive/migration.
- Flexible scheduling: manual runs, minute-based intervals, Cron schedules, and one-click execution of all enabled jobs.
- Precise filtering: Gitignore-style exclusion rules and minimum/maximum file size filters.
- Realtime progress: scanned counts, transfer speed, remaining time, and per-file details for done, failed, waiting, and running files.
- Task management: stop running jobs; view details, retry unfinished items, and delete history records.
- Multiple engines: manage multiple AList / OpenList instances; addresses and tokens are validated before saving, and saved tokens are never echoed back.
- Notification channels: custom Webhook, Server酱, DingTalk, WeCom, and Feishu / Lark, with an option to stay silent when there is nothing to sync.
- Account recovery: a 24-digit recovery key generated on first setup, web password reset, and a `reset-password` CLI fallback.
- Adaptive UI: desktop and mobile layouts with a consistent light theme.

## Configuration

The timezone of scheduled tasks is always controlled by `TZ`. When `data/config.ini` does not exist, other startup settings are read from environment variables:

| Variable | Default | Description |
| --- | --- | --- |
| `TZ` | `Asia/Shanghai` | Timezone used by the container and scheduled tasks |
| `PUID` / `PGID` | `1000` / `1000` | Host user and group the container runs as |
| `OPENSYNC_BIND` | `0.0.0.0` | HTTP listen address |
| `OPENSYNC_PORT` | `8023` | HTTP service port |
| `OPENSYNC_EXPIRES` | `7` | Login validity, in days |
| `OPENSYNC_LOG_LEVEL` | `1` | File log level |
| `OPENSYNC_CONSOLE_LEVEL` | `2` | Console log level |
| `OPENSYNC_LOG_SAVE` | `7` | Log retention days |
| `OPENSYNC_TASK_SAVE` | `30` | History retention days, `0` keeps all |
| `OPENSYNC_TASK_TIMEOUT` | `48` | Per-task timeout in hours, `0` means unlimited |
| `OPENSYNC_COPY_CONCURRENCY` | `5` | Copy concurrency per task, range `1` to `100` |
| `OPENSYNC_SCAN_CONCURRENCY` | `8` | Scan concurrency per task, range `1` to `20` |
| `OPENSYNC_MAX_RETRIES` | `2` | Max auto-retries after a copy item fails, `0` disables auto-retry |
| `OPENSYNC_TRUSTED_PROXIES` | empty | Comma-separated CIDR list of trusted reverse proxies |
| `OPENSYNC_CHOWN` | auto | Data directory ownership policy: `always`, `never`, or unset |

Ports: publish `8023` (or whatever you set `OPENSYNC_PORT` to).

> Note: once `data/config.ini` exists, environment variables other than `OPENSYNC_TLS_CERT` / `OPENSYNC_TLS_KEY` no longer take effect after a restart — the config file wins. History retention, task timeout, copy/scan concurrency, and retry counts can also be adjusted online from the System Settings page.

## Data Persistence

Mount `/app/data` — it holds the database, secret key, config, and logs. Losing it means losing your account, engines, jobs, and history.

- Docker Compose uses `./data:/app/data`; the example above uses the named volume `opensync-data:/app/data`.
- At container startup the ownership of `/app/data` is checked against `PUID` / `PGID` so no root-owned files appear in the host directory. Force a recursive chown with `OPENSYNC_CHOWN=always` or skip it with `OPENSYNC_CHOWN=never`.
- Do not delete `data/secret.key` when upgrading — it backs login cookies and the encryption of sensitive data. Web password reset only uses the recovery key and never exposes `secret.key`.
- Back up the mounted `data/` directory before upgrading. Database migrations run automatically on first start.
- Never expose the `data/` directory or any file containing AList / OpenList tokens; rotate your tokens if the runtime data directory was shared accidentally.

## Links

- GitHub: https://github.com/chenbin3625/OpenSync
- Releases: https://github.com/chenbin3625/OpenSync/releases

---

# 中文

OpenSync 是面向飞牛 fnOS / 飞牛 NAS、普通 NAS 和 Docker 环境的 AList / OpenList 自动同步工具。它通过 AList / OpenList 连接本地目录、网盘、对象存储、WebDAV 等存储端，并用可视化任务完成备份、镜像、归档和迁移。

![任务管理](https://raw.githubusercontent.com/chenbin3625/OpenSync/main/docs/images/tasks-overview.png)

## 快速部署

```bash
docker run -d \
  --name opensync \
  --restart unless-stopped \
  -p 8023:8023 \
  -v opensync-data:/app/data \
  -e TZ=Asia/Shanghai \
  -e OPENSYNC_BIND=0.0.0.0 \
  -e OPENSYNC_PORT=8023 \
  -e GIN_MODE=release \
  chenbin3625/opensync:latest
```

启动后访问 `http://你的设备IP:8023/`，按提示创建管理员用户名和密码。

首次启动时会一次性展示 24 位恢复密钥，请立即保存。忘记密码时可用“用户名 + 恢复密钥”在 Web 端设置新密码（旧恢复密钥失效并生成新的）。如果恢复密钥也丢失，执行 CLI 兜底重置：

```bash
docker exec -it opensync ./opensync reset-password --user admin
```

推荐使用 Docker Compose 部署：

```bash
mkdir -p opensync
cd opensync
curl -O https://raw.githubusercontent.com/chenbin3625/OpenSync/main/docker-compose.yml
docker compose up -d
```

## 镜像标签

| 标签 | 说明 |
| --- | --- |
| `chenbin3625/opensync:latest` | 最新稳定版，默认推荐 |
| `chenbin3625/opensync:1.13.2` | 固定完整版本号，适合需要可复现部署的场景 |
| `chenbin3625/opensync:1.13` | 次版本号标签，跟随最新的 `1.13.x` 补丁版本 |
| `chenbin3625/opensync:1.10` | 较早的次版本号标签，供已有部署继续使用 |

每次 GitHub 正式发布都会由 Docker Hub 工作流推送镜像，同时发布完整版本号、次版本号（如 `1.13`）和 `latest`（预发布版本不会更新 `latest`）。

镜像支持以下平台：

- `linux/amd64`
- `linux/arm64`
- `linux/arm/v7`

## 重点功能

- 多源多目标：支持单个或多个源目录同步到单个或多个目标目录。
- 三种同步模式：仅新增、全同步和移动模式，分别适合增量备份、目标镜像和归档迁移。
- 灵活调度：支持手动执行、按分钟间隔执行、Cron 定时执行，以及一键执行全部已启用任务。
- 精确筛选：支持 Gitignore 风格排除规则和最小/最大文件大小过滤。
- 实时进度：展示扫描数量、传输速度、剩余时间，以及已完成、失败、等待和运行中的文件明细。
- 任务管理：运行中的任务可以停止；历史任务可以查看详情、重试未完成项和删除记录。
- 多引擎：可管理多个 AList / OpenList 实例，保存前验证地址和令牌，已保存令牌不会回显。
- 通知渠道：支持自定义 Webhook、Server 酱、钉钉、企业微信和飞书 / Lark，可设置无文件需要同步时静默。
- 账号恢复：首次初始化生成 24 位恢复密钥，支持网页重置密码和 `reset-password` CLI 兜底重置。
- 自适应界面：支持桌面端和移动端布局，界面保持统一的浅色主题。

## 配置

定时任务时区始终由 `TZ` 控制；当 `data/config.ini` 不存在时，其它启动配置会读取环境变量：

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `TZ` | `Asia/Shanghai` | 容器和定时任务使用的时区 |
| `PUID` / `PGID` | `1000` / `1000` | 容器运行使用的宿主机用户和用户组 |
| `OPENSYNC_BIND` | `0.0.0.0` | HTTP 监听地址 |
| `OPENSYNC_PORT` | `8023` | HTTP 服务端口 |
| `OPENSYNC_EXPIRES` | `7` | 登录有效期，单位天 |
| `OPENSYNC_LOG_LEVEL` | `1` | 文件日志等级 |
| `OPENSYNC_CONSOLE_LEVEL` | `2` | 控制台日志等级 |
| `OPENSYNC_LOG_SAVE` | `7` | 日志保留天数 |
| `OPENSYNC_TASK_SAVE` | `30` | 历史任务保留天数，`0` 表示保留全部 |
| `OPENSYNC_TASK_TIMEOUT` | `48` | 单次任务超时时间，单位小时，`0` 表示不限制 |
| `OPENSYNC_COPY_CONCURRENCY` | `5` | 单个任务的复制并发数，范围 `1` 到 `100` |
| `OPENSYNC_SCAN_CONCURRENCY` | `8` | 单个任务的扫描并发数，范围 `1` 到 `20` |
| `OPENSYNC_MAX_RETRIES` | `2` | 单个复制项失败后的最大自动重试次数，`0` 表示不自动重试 |
| `OPENSYNC_TRUSTED_PROXIES` | 空 | 信任的反向代理 CIDR 列表（逗号分隔） |
| `OPENSYNC_CHOWN` | 自动 | 数据目录权限策略：`always`、`never`，或未设置时自动判断 |

端口：映射 `8023`（或你设置的 `OPENSYNC_PORT`）。

> 注意：`data/config.ini` 一旦存在，除 `OPENSYNC_TLS_CERT` / `OPENSYNC_TLS_KEY` 外的环境变量在重启后将不再生效，配置文件优先。历史任务保留、任务超时、复制/扫描并发和自动重试次数也可以在系统设置页在线调整。

## 数据持久化

请持久化 `/app/data`，它包含数据库、密钥、配置和日志。丢失该目录意味着丢失账号、引擎、任务和历史记录。

- Docker Compose 使用 `./data:/app/data`；上面的命令示例使用命名卷 `opensync-data:/app/data`。
- 容器启动时会根据 `PUID` / `PGID` 检查 `/app/data` 的文件归属，避免宿主机目录生成 root 权限文件。可通过 `OPENSYNC_CHOWN=always` 强制递归修改，或通过 `OPENSYNC_CHOWN=never` 跳过。
- 升级时不要删除 `data/secret.key`，否则旧登录 Cookie 和敏感信息加解密会失效。Web 端密码重置只使用恢复密钥，不会暴露 `secret.key`。
- 升级前建议先备份挂载的 `data/` 目录，首次启动会自动执行数据库迁移。
- 不要公开 `data/` 目录或任何包含 AList / OpenList Token 的文件；如果误分享了运行数据目录，请及时更换 Token。

## 项目地址

- GitHub: https://github.com/chenbin3625/OpenSync
- Releases: https://github.com/chenbin3625/OpenSync/releases
