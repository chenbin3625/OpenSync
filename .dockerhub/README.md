# OpenSync

OpenSync is an AList / OpenList automation layer for fnOS (FeiNiu NAS), general NAS, and Docker environments. It connects local folders, cloud drives, object storage, WebDAV, and other storage backends through AList / OpenList, then uses visual jobs to handle backup, mirror, archive, and migration workflows.

If you are on fnOS and want a Cloud Sync-style experience for a photo library, media library, download folder, or document folder, OpenSync is built for that exact gap.

![Task Management](https://raw.githubusercontent.com/chenbin3625/OpenSync/main/docs/images/tasks-overview.png)

## Quick Start

Docker Compose is recommended:

```bash
mkdir -p opensync
cd opensync
curl -O https://raw.githubusercontent.com/chenbin3625/OpenSync/main/docker-compose.yml
docker compose up -d
```

Then open `http://<your-device-ip>:8023/` and follow the prompts to create an admin username and password.

On first start, a 24-digit recovery key is shown once. Save it immediately. If you forget the password, set a new one on the web page with "username + recovery key"; the old key becomes invalid and a new one is generated. If the recovery key is also lost, run the CLI fallback:

```bash
[ -f .env ] && . ./.env
docker compose exec --user "${PUID:-1000}:${PGID:-1000}" opensync ./opensync reset-password --user admin
```

Docker CLI deployment:

```bash
docker run -d \
  --name opensync \
  --restart unless-stopped \
  -p 8023:8023 \
  -v opensync-data:/app/data \
  --tmpfs /tmp:mode=1777 \
  -e TZ=Asia/Shanghai \
  -e SQLITE_TMPDIR=/tmp \
  -e TMPDIR=/tmp \
  -e OPENSYNC_BIND=0.0.0.0 \
  -e OPENSYNC_PORT=8023 \
  -e GIN_MODE=release \
  chenbin3625/opensync:latest
```

## Image Tags

| Tag | Description |
| --- | --- |
| `chenbin3625/opensync:latest` | Latest stable release, recommended by default |
| `chenbin3625/opensync:1.13.2` | Pinned full version, for reproducible deployments |
| `chenbin3625/opensync:1.13` | Minor-version tag, tracks the newest `1.13.x` patch release |
| `chenbin3625/opensync:1.10` | Older minor-version tag kept for existing deployments |

Images are published on every official GitHub release by the Docker Hub workflow. Stable releases publish the full version, the minor version, and `latest`; prereleases only publish their exact version tag.

Supported platforms:

- `linux/amd64`
- `linux/arm64`
- `linux/arm/v7`

## Typical Scenarios

| Scenario | Example |
| --- | --- |
| fnOS photo backup | Back up a local photo library to a cloud drive or object storage bucket. |
| Download archive | Move completed downloads to a WebDAV drive, another NAS, or a cold-storage folder. |
| Media library mirror | Mirror a media folder from one AList / OpenList backend to another. |
| Document protection | Run add-only backups for work documents and notify only when files changed. |
| Storage migration | Move or mirror files between different cloud drives exposed through AList / OpenList. |

## Key Features

- Multiple sources and targets: sync one or more source directories to one or more target directories.
- Three sync modes: Add-only, Full sync, and Move for incremental backup, target mirroring, and archive/migration.
- Flexible scheduling: manual runs, minute-based intervals, Cron schedules, and one-click execution of all enabled jobs.
- Precise filtering: Gitignore-style exclusion rules and minimum/maximum file size filters.
- Realtime progress: scanned counts, transfer speed, remaining time, and per-file details for done, failed, waiting, and running files.
- Task management: stop running jobs; view details, retry unfinished items, and delete history records.
- Multiple engines: manage multiple AList / OpenList instances; addresses and tokens are validated before saving, and saved tokens are never echoed back.
- Notification channels: custom Webhook, ServerChan, DingTalk, WeCom, and Feishu / Lark, with an option to stay silent when there is nothing to sync.
- Account recovery: a 24-digit recovery key generated on first setup, web password reset, and a `reset-password` CLI fallback.
- Adaptive UI: desktop and mobile layouts with a consistent light theme.

## Configuration

The timezone of scheduled tasks is always controlled by `TZ`. When `data/config.ini` does not exist, other startup settings are read from environment variables:

| Variable | Default | Description |
| --- | --- | --- |
| `TZ` | `Asia/Shanghai` | Timezone used by the container and scheduled tasks |
| `PUID` / `PGID` | `1000` / `1000` | Host user and group the container runs as |
| `SQLITE_TMPDIR` / `TMPDIR` | `/tmp` | Temporary directory used by SQLite and runtime operations |
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

Ports: publish `8023` or whatever you set `OPENSYNC_PORT` to.

> Note: once `data/config.ini` exists, environment variables other than `OPENSYNC_TLS_CERT` / `OPENSYNC_TLS_KEY` no longer take effect after a restart. The config file wins. History retention, task timeout, copy/scan concurrency, and retry counts can also be adjusted online from the System Settings page.

## Data Persistence

Mount `/app/data`. It holds the database, secret key, config, and logs. Losing it means losing your account, engines, jobs, and history.

- Docker Compose uses `./data:/app/data`; the Docker CLI example uses the named volume `opensync-data:/app/data`.
- At container startup the ownership of `/app/data` is checked against `PUID` / `PGID` so no root-owned files appear in the host directory. Force a recursive chown with `OPENSYNC_CHOWN=always` or skip it with `OPENSYNC_CHOWN=never`.
- Do not delete `data/secret.key` when upgrading. It backs login cookies and the encryption of sensitive data. Web password reset only uses the recovery key and never exposes `secret.key`.
- Back up the mounted `data/` directory before upgrading. Database migrations run automatically on first start.
- Never expose the `data/` directory or any file containing AList / OpenList tokens; rotate your tokens if the runtime data directory was shared accidentally.

## Links

- GitHub: https://github.com/chenbin3625/OpenSync
- Releases: https://github.com/chenbin3625/OpenSync/releases
- Chinese README: https://github.com/chenbin3625/OpenSync/blob/main/README.zh-CN.md

---

# 中文

OpenSync 是面向飞牛 fnOS / 飞牛 NAS、普通 NAS 和 Docker 环境的 AList / OpenList 自动同步工具。它通过 AList / OpenList 连接本地目录、网盘、对象存储、WebDAV 等存储端，并用可视化任务完成备份、镜像、归档和迁移。

如果你在飞牛 NAS 上想找一个类似群晖 Cloud Sync 的同步工具，用来把照片库、影音库、下载目录或文档目录同步到网盘、对象存储或另一台存储设备，OpenSync 就是面向这个场景做的。

![任务管理](https://raw.githubusercontent.com/chenbin3625/OpenSync/main/docs/images/tasks-overview.png)

## 快速部署

推荐使用 Docker Compose：

```bash
mkdir -p opensync
cd opensync
curl -O https://raw.githubusercontent.com/chenbin3625/OpenSync/main/docker-compose.yml
docker compose up -d
```

启动后访问 `http://你的设备IP:8023/`，按提示创建管理员用户名和密码。

首次启动时会一次性展示 24 位恢复密钥，请立即保存。忘记密码时可用“用户名 + 恢复密钥”在 Web 端设置新密码，旧恢复密钥会失效并生成新的。如果恢复密钥也丢失，可执行 CLI 兜底重置：

```bash
[ -f .env ] && . ./.env
docker compose exec --user "${PUID:-1000}:${PGID:-1000}" opensync ./opensync reset-password --user admin
```

## 镜像标签

| 标签 | 说明 |
| --- | --- |
| `chenbin3625/opensync:latest` | 最新稳定版，默认推荐 |
| `chenbin3625/opensync:1.13.2` | 固定完整版本号，适合需要可复现部署的场景 |
| `chenbin3625/opensync:1.13` | 次版本号标签，跟随最新的 `1.13.x` 补丁版本 |
| `chenbin3625/opensync:1.10` | 较早的次版本号标签，供已有部署继续使用 |

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
- 自适应界面：支持桌面端和移动端布局，界面保持统一浅色主题。

更多说明见 GitHub README：

https://github.com/chenbin3625/OpenSync/blob/main/README.zh-CN.md
