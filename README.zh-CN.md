# OpenSync

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE) [![Release](https://img.shields.io/github/v/release/chenbin3625/OpenSync)](https://github.com/chenbin3625/OpenSync/releases) [![CI](https://github.com/chenbin3625/OpenSync/actions/workflows/ci.yml/badge.svg)](https://github.com/chenbin3625/OpenSync/actions/workflows/ci.yml) [![Docker Pulls](https://img.shields.io/docker/pulls/chenbin3625/opensync)](https://hub.docker.com/r/chenbin3625/opensync)

[官网](https://opensync.u1n1.com/) | [English](README.md) | [60 秒演示动图](docs/demo/opensync-demo.gif) | [Docker Hub](https://hub.docker.com/r/chenbin3625/opensync) | [Discussions](https://github.com/chenbin3625/OpenSync/discussions)

OpenSync 是面向飞牛 fnOS / 飞牛 NAS、普通 NAS 和 Docker 环境的 AList / OpenList 自动同步工具。它通过 AList / OpenList 连接本地目录、网盘、对象存储、WebDAV 等存储端，并用可视化任务完成备份、镜像、归档和迁移。

如果你在飞牛 NAS 上想找一个类似群晖 Cloud Sync 的同步工具，用来把照片库、影音库、下载目录或文档目录同步到网盘、对象存储或另一台存储设备，OpenSync 就是面向这个场景做的。

如果 OpenSync 正好补上了你的 NAS 自动同步缺口，欢迎在 [GitHub](https://github.com/chenbin3625/OpenSync) 点一个 Star，帮助更多飞牛、NAS、Docker、AList / OpenList 用户发现它。

![OpenSync 任务总览](docs/images/tasks-overview.png)

## 为什么做 OpenSync

- 你已经用 AList / OpenList 接好了各种存储端，但还缺少定时备份、迁移和归档任务。
- 你想把 NAS 本地目录同步到网盘、对象存储、WebDAV 或另一台设备，不想每次手动复制。
- 你希望看到实时进度、失败文件、历史记录和失败重试，而不是只看命令行日志。
- 你需要一个适合 Docker、飞牛 NAS、普通 NAS 和 ARM 设备的可视化同步工具。

## 60 秒演示

这张动图展示 OpenSync 的主流程：添加 AList / OpenList 引擎、新建同步任务、手动运行、查看实时进度和接收通知。

![OpenSync 60 秒演示](docs/demo/opensync-demo.gif)

## 快速部署

推荐使用 Docker Compose 部署：

```bash
mkdir -p opensync
cd opensync
curl -O https://raw.githubusercontent.com/chenbin3625/OpenSync/main/docker-compose.yml
docker compose up -d
```

启动后访问：

```text
http://你的设备IP:8023/
```

首次启动后，打开 Web 页面按提示创建管理员用户名和密码。创建完成时页面会一次性展示 24 位恢复密钥，请立即保存；忘记密码时可用“用户名 + 恢复密钥”在 Web 端设置新密码，成功后旧恢复密钥会失效并生成新的恢复密钥。

如果恢复密钥也丢失，需要登录服务器执行 CLI 兜底重置：

```bash
./opensync reset-password --user admin
```

Docker Compose 部署可执行：

```bash
[ -f .env ] && . ./.env
docker compose exec --user "${PUID:-1000}:${PGID:-1000}" opensync ./opensync reset-password --user admin
```

## 典型场景

| 场景 | 示例 |
| --- | --- |
| 飞牛相册备份 | 把本地照片库自动备份到网盘或对象存储。 |
| 下载目录归档 | 把下载完成的文件定时移动到 WebDAV、另一台 NAS 或冷存储目录。 |
| 影音库镜像 | 把一个 AList / OpenList 后端中的媒体目录镜像到另一个后端。 |
| 文档保护 | 对工作文档做仅新增备份，并在确实有文件同步时通知。 |
| 存储迁移 | 在 AList / OpenList 暴露的不同网盘之间迁移或镜像文件。 |

## OpenSync 与其它方案对比

| 能力 | OpenSync | rclone | 群晖 Cloud Sync | AList / OpenList 自带复制 |
| --- | --- | --- | --- | --- |
| 目标用户 | 飞牛、NAS、Docker、AList / OpenList 用户 | 命令行和脚本用户 | 群晖用户 | AList / OpenList 用户 |
| 可视化任务管理 | 支持 | 不支持，除非额外封装 UI | 支持 | 偏手动操作 |
| AList / OpenList 集成 | 原生管理多个引擎 | 通常通过 WebDAV 或配置文件 | 不面向 AList / OpenList | 原生，但不是调度器 |
| 多源多目标任务 | 支持 | 可脚本化 | 受产品能力限制 | 手动操作为主 |
| 仅新增 / 全同步 / 移动 | 支持 | 支持，依赖命令组合 | 受产品能力限制 | 复制 / 移动为主 |
| 实时逐文件进度 | 支持 | 命令行输出或日志 | 产品 UI | 有限 |
| 历史任务失败重试 | 支持 | 可脚本化 | 受产品能力限制 | 无专门历史重试 |
| 通知渠道 | Webhook、Server 酱、钉钉、企业微信、飞书 | 可脚本化 | 受产品能力限制 | 无专门通知层 |
| 最适合 | 给 AList / OpenList 加一层可视化 Cloud Sync | 高级用户和自动化脚本 | 只用群晖生态的场景 | 偶尔手动复制文件 |

## 重点功能

- 多源多目标：支持单个或多个源目录同步到单个或多个目标目录。
- 三种同步模式：仅新增、全同步和移动模式，分别适合增量备份、目标镜像和归档迁移。
- 灵活调度：支持手动执行、按分钟间隔执行、Cron 定时执行，以及一键执行全部已启用任务。
- 精确筛选：支持 Gitignore 风格排除规则和最小/最大文件大小过滤。
- 实时进度：展示扫描数量、传输速度、剩余时间，以及已完成、失败、等待和运行中的文件明细。
- 任务管理：运行中的任务可以停止；历史任务可以查看详情、重试未完成项和删除记录。
- 多引擎：可管理多个 AList / OpenList 实例，保存前验证地址和令牌，已保存令牌不会回显。
- 通知渠道：支持自定义 Webhook、Server 酱、钉钉、企业微信和飞书 / Lark；可设置无文件需要同步时静默。
- Webhook 定制：支持 GET / POST / PUT、JSON 请求体模板、自定义请求头和发送测试。
- 在线运行配置：可调整历史任务保留时间、任务超时、复制/扫描并发和失败自动重试次数。
- 账号恢复：首次初始化生成 24 位恢复密钥，支持网页重置密码和 `reset-password` CLI 兜底重置。
- 自适应界面：支持桌面端和移动端布局，保持统一浅色主题，任务结束后实时视图会立即更新。

## 界面预览

### 任务管理

任务以卡片形式展示源目录、目标目录、同步模式与调度计划，可实时查看统计并一键执行、停止、编辑或删除。

![任务管理](docs/images/tasks-overview.png)

### 实时任务

任务运行时可实时查看扫描文件数、传输速度、剩余时间，以及已完成、失败、等待和运行中的文件明细。

![实时任务](docs/images/realtime-task.png)

### 历史任务

历史任务记录每次执行的开始时间、耗时与结果，可查看详情、重试未完成项或删除记录。

![历史任务](docs/images/task-history.png)

### 任务详情

任务详情页汇总扫描结果与传输统计，并可逐条查看每个文件的同步状态与错误信息。

![任务详情](docs/images/task-detail.png)

### 引擎管理

可维护多个 AList / OpenList 实例，添加引擎时自动验证地址与令牌，已保存令牌不会回显。

![引擎管理](docs/images/engines.png)

### 通知配置

支持自定义 Webhook、Server 酱、钉钉、企业微信和飞书 / Lark 等通知渠道，保存前可发送测试消息。

![通知配置](docs/images/notifications.png)

### 系统设置

在线调整历史任务保留时间、任务超时、复制/扫描并发与失败自动重试，并支持账号与密码管理。

![系统设置](docs/images/settings.png)

## 自定义 Webhook 通知

在通知配置页新增通知时，选择“自定义Webhook”即可接入支持 HTTP/HTTPS 回调的消息服务或自动化平台。

- `URL` 为必填项，必须填写有效的 HTTP 或 HTTPS Webhook 地址，可指向内网或本机服务。
- `HTTP方法` 支持 `GET`、`POST`、`PUT`，默认使用 `POST`。
- `GET` 会把通知标题和内容作为 `title`、`content` 查询参数发送。
- `POST` / `PUT` 默认以 `application/json` 发送请求体。
- `请求体模板` 可选，必须是 JSON 对象，支持 `{title}` 和 `{content}` 占位符；留空时默认发送 `{"title":"通知标题","content":"通知内容"}`。
- `请求头 JSON` 可选，必须是 JSON 对象，适合填写 `Authorization`、`X-Token` 等鉴权头。
- 打开“无需同步时不发送”后，当任务没有需要同步的内容时不会发送通知。
- 保存前可以点击“测试”发送测试消息，确认目标服务能够正常接收。

示例请求体模板：

```json
{
  "msg_type": "text",
  "text": {
    "title": "{title}",
    "content": "{content}"
  }
}
```

示例请求头：

```json
{
  "Authorization": "Bearer your-token"
}
```

## docker-compose.yml

```yaml
services:
  opensync:
    image: chenbin3625/opensync:latest
    container_name: opensync
    restart: unless-stopped
    ports:
      - "8023:8023"
    volumes:
      - ./data:/app/data
    tmpfs:
      - /tmp:mode=1777
    environment:
      TZ: Asia/Shanghai
      PUID: ${PUID:-1000}
      PGID: ${PGID:-1000}
      SQLITE_TMPDIR: /tmp
      TMPDIR: /tmp
      OPENSYNC_BIND: 0.0.0.0
      OPENSYNC_PORT: 8023
      GIN_MODE: release
```

每次发布同时推送版本号标签和 `latest`，`latest` 始终指向最新的稳定版本。

## Docker 命令部署

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

## 升级说明

1. 备份当前挂载的 `data/` 目录。
2. 拉取最新镜像或指定版本镜像。
3. 重新启动容器。
4. 首次启动会自动执行数据库迁移。

升级时不要删除 `data/secret.key`，否则旧登录 Cookie 和敏感信息加解密会失效。Web 端密码重置只使用恢复密钥，不会要求输入或暴露 `data/secret.key`。

## 配置

定时任务时区始终由 `TZ` 控制；当 `data/config.ini` 不存在时，其它启动配置会读取环境变量：

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `TZ` | `Asia/Shanghai` | 容器和定时任务使用的时区 |
| `PUID` / `PGID` | `1000` / `1000` | 容器运行使用的宿主机用户和用户组 |
| `SQLITE_TMPDIR` / `TMPDIR` | `/tmp` | SQLite 和运行时使用的临时目录 |
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
| `OPENSYNC_CHOWN` | 自动 | 容器数据目录权限策略：`always` 强制递归修改，`never` 跳过，未设置时仅在目录所有者不匹配时修改 |

端口：映射 `8023`，或你设置的 `OPENSYNC_PORT`。

> 注意：`data/config.ini` 一旦存在，除 `OPENSYNC_TLS_CERT` / `OPENSYNC_TLS_KEY` 外的环境变量在重启后将不再生效，配置文件优先。历史任务保留、任务超时、复制/扫描并发和自动重试次数也可以在系统设置页在线调整。

## 数据持久化

请持久化 `/app/data`，它包含数据库、密钥、配置和日志。丢失该目录意味着丢失账号、引擎、任务和历史记录。

- Docker Compose 使用 `./data:/app/data`；Docker 命令示例使用命名卷 `opensync-data:/app/data`。
- 容器启动时会根据 `PUID` / `PGID` 检查 `/app/data` 的文件归属，避免宿主机目录生成 root 权限文件。可通过 `OPENSYNC_CHOWN=always` 强制递归修改，或通过 `OPENSYNC_CHOWN=never` 跳过。
- 升级时不要删除 `data/secret.key`，否则旧登录 Cookie 和敏感信息加解密会失效。Web 端密码重置只使用恢复密钥，不会暴露 `secret.key`。
- 升级前建议先备份挂载的 `data/` 目录，首次启动会自动执行数据库迁移。
- 不要公开 `data/` 目录或任何包含 AList / OpenList Token 的文件；如果误分享了运行数据目录，请及时更换 Token。

## 本地构建镜像

```bash
docker build -t opensync .
docker run -d \
  --name opensync \
  --restart unless-stopped \
  -p 8023:8023 \
  -v opensync-data:/app/data \
  --tmpfs /tmp:mode=1777 \
  -e TZ=Asia/Shanghai \
  -e OPENSYNC_PORT=8023 \
  -e GIN_MODE=release \
  opensync
```

## 不使用 Docker 的生产构建

先构建前端，构建结果会写入 Go 的静态资源嵌入目录：

```bash
cd frontend
npm install
npm run build
```

再构建并运行后端：

```bash
cd ../backend
go build -o opensync ./cmd/server
./opensync
```

## 本地开发

启动后端：

```bash
cd backend
go run ./cmd/server
```

启动前端开发服务：

```bash
cd frontend
npm install
npm run dev
```

前端开发服务地址：

```text
http://127.0.0.1:3000/
```

开发服务会把 `/svr` 接口代理到：

```text
http://localhost:8023
```

## 开发检查

```bash
cd frontend
npm run build

cd ../backend
go test ./...
```

## 社区讨论

GitHub Discussions 已开启：

- `Q&A`：使用求助、部署排障和配置问题。
- `Ideas`：功能建议、改进想法和路线讨论。
- `Show and tell`：部署案例、截图、NAS 方案分享。

可复现的问题请提交 Issue；使用问题和部署经验优先发 Discussions。

## 安全

不要提交或公开 `backend/data`、Docker 挂载的 `data/` 目录或任何包含 AList / OpenList Token 的文件。漏洞反馈和加固建议见 [SECURITY.md](SECURITY.md)。

## 贡献

欢迎提交 Issue、Discussion 和 Pull Request。提交代码前请阅读 [CONTRIBUTING.md](CONTRIBUTING.md)。

## 许可证

本项目以 [MIT License](LICENSE) 授权。
