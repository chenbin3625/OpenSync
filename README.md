# OpenSync

OpenSync 是一个面向 AList / OpenList 的自动化同步工具，用来统一管理存储引擎、创建目录同步任务、跟踪实时进度，并在任务结束后发送通知。

它适合部署在飞牛 fnOS、NAS、家庭服务器和普通 Docker 环境中。通过 AList / OpenList 接入网盘、对象存储、WebDAV、本地目录等存储端后，OpenSync 可以按手动、间隔或 Cron 策略执行同步任务，让备份、归档和迁移过程更可控。

## 1.4.0 更新重点

- 重做任务管理页，改为左侧任务列表和右侧任务工作台，支持总览、实时任务、历史任务三个视图。
- 新增运行中任务控制，可以在实时任务页暂停当前任务，也可以从历史任务继续执行或重试失败项。
- 新增失败项重试，支持只重试某次任务中的失败文件或目录，避免每次都重新扫描全量任务。
- 新增文件大小过滤，任务可设置最小文件大小和最大文件大小，支持 B、KB、MB、GB、TB 单位。
- 新增系统设置页，可在界面调整登录有效期、历史任务保留、任务超时、复制并发、扫描并发、实时明细保留数和最大自动重试次数。
- 新增复制失败自动重试，单个复制项失败后可按配置自动重试，并在任务明细中保留重试状态。
- 优化实时进度展示，任务列表会自动合并刷新结果，减少运行中任务闪动和历史任务重复展示。
- 优化顶部导航和移动端布局，界面更适合桌面、平板和小屏设备。
- 优化通知发送错误处理，Webhook、Server 酱、钉钉、企业微信、飞书等渠道会更明确地暴露 HTTP 错误。
- 优化登录态和接口错误响应，未登录和登录过期使用标准 401，服务端异常使用标准 500。
- 优化数据库迁移，新增任务文件大小字段，并避免重复迁移导致升级失败。

## 界面预览

### 登录

![登录](docs/images/login.png)

### 任务总览

![任务总览](docs/images/tasks-overview.png)

### 实时任务

![实时任务](docs/images/realtime-task.png)

### 历史任务

![历史任务](docs/images/task-history.png)

### 新建和编辑任务

![新建和编辑任务](docs/images/job-form.png)

### 引擎管理

![引擎管理](docs/images/engines.png)

### 通知配置

![通知配置](docs/images/notifications.png)

### 系统设置

![系统设置](docs/images/settings.png)

## 适合场景

- 在飞牛 fnOS 或家庭 NAS 上，把本地目录定时备份到 AList / OpenList 支持的远端存储。
- 替代或补充群晖 Cloud Sync，完成跨网盘、跨对象存储、跨 WebDAV 的同步。
- 为照片库、影音库、文档目录、下载目录配置自动备份或归档。
- 管理多个 AList / OpenList 引擎和多条同步任务。
- 查看每次同步的成功、失败、进度、速度、剩余量和错误原因。
- 对失败项单独重试，减少大任务失败后的重复扫描和重复复制。
- 在同步完成、失败或无需同步时，通过消息渠道推送结果。

## 功能总览

### 引擎管理

- 添加、编辑和删除 AList / OpenList 引擎。
- 支持引擎地址、备注和令牌管理。
- 添加或更新引擎时会验证连接，列表中不会展示令牌。
- 删除引擎前会检查是否仍被同步任务使用，避免误删运行依赖。

### 同步任务

- 支持单源目录、多个源目录、单目标目录和多个目标目录。
- 支持三种同步方式：
  - 仅新增：复制源端新增或变更文件，不删除目标端多余文件。
  - 全同步：让目标端尽量与源端一致，源端不存在的目标文件会被删除。
  - 移动模式：按迁移任务处理新增或变更文件，适合归档或腾挪空间。
- 支持三种调度方式：
  - 手动执行。
  - 按分钟间隔执行。
  - Cron 表达式执行。
- 支持源端缓存、目标端缓存和扫描间隔配置。
- 支持 Gitignore 风格排除规则，默认带有 macOS、Windows、Linux、NAS、临时文件和缓存目录规则。
- 支持按文件大小过滤，可设置最小和最大文件大小。
- 支持启用、禁用、编辑、删除和手动执行任务。
- 支持执行全部启用任务。

### 任务执行和历史

- 实时展示目录扫描、耗时、平均速度、瞬时速度、预计剩余、已传输和剩余大小。
- 实时明细按等待、运行中、成功、失败和其他状态分组。
- 历史任务展示状态、开始时间、成功数、失败数和总数。
- 支持查看任务详情，按状态、类型、路径和错误信息筛选明细。
- 支持暂停运行中任务。
- 支持从历史任务继续执行。
- 支持只重试失败项。
- 支持删除历史任务。
- 支持任务超时、异常任务恢复和历史任务自动清理。

### 通知

- 支持自定义 Webhook。
- 支持 Server 酱。
- 支持钉钉。
- 支持企业微信。
- 支持飞书 / Lark。
- 支持通知测试。
- 支持配置“无需同步时不发送”。
- 通知发送失败会显示更明确的 HTTP 响应错误。

### 系统和安全

- 支持登录、登出、忘记密码、修改密码。
- 支持深色模式。
- 支持中英文语言。
- 登录 Cookie 使用持久化密钥签名，部署后请保留 `data/secret.key`。
- 未登录和登录过期会返回标准 401，前端会自动回到登录页。
- 本地数据使用 SQLite 保存。
- 支持通过界面保存运行配置，配置会写入 `data/config.ini`。

## Docker Compose 部署

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

首次启动时，初始管理员密码会打印在容器日志里：

```bash
docker logs opensync
```

默认配置会把运行数据保存到当前目录的 `data/` 文件夹。请保留这个目录，它包含数据库、密钥、配置和日志。

## docker-compose.yml

```yaml
services:
  opensync:
    image: ghcr.io/chenbin3625/opensync:latest
    container_name: opensync
    restart: unless-stopped
    ports:
      - "8023:8023"
    volumes:
      - ./data:/app/data
    environment:
      OPENSYNC_PORT: 8023
      GIN_MODE: release
```

如需固定版本，可以把镜像改为：

```yaml
image: ghcr.io/chenbin3625/opensync:1.4.0
```

## Docker 命令部署

```bash
docker run -d \
  --name opensync \
  --restart unless-stopped \
  -p 8023:8023 \
  -v opensync-data:/app/data \
  -e OPENSYNC_PORT=8023 \
  -e GIN_MODE=release \
  ghcr.io/chenbin3625/opensync:latest
```

## 升级说明

1. 备份当前挂载的 `data/` 目录。
2. 拉取最新镜像或指定 `1.4.0` 镜像。
3. 重新启动容器。
4. 首次启动会自动执行数据库迁移，新增任务文件大小过滤字段。

升级时不要删除 `data/secret.key`，否则旧登录 Cookie 和加密数据会失效。

## 配置

当 `data/config.ini` 不存在时，会读取环境变量：

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `OPENSYNC_PORT` | `8023` | HTTP 服务端口 |
| `OPENSYNC_EXPIRES` | `7` | 登录有效期，单位天 |
| `OPENSYNC_LOG_LEVEL` | `1` | 文件日志等级 |
| `OPENSYNC_CONSOLE_LEVEL` | `2` | 控制台日志等级 |
| `OPENSYNC_LOG_SAVE` | `7` | 日志保留天数 |
| `OPENSYNC_TASK_SAVE` | `30` | 历史任务保留天数，`0` 表示保留全部 |
| `OPENSYNC_TASK_TIMEOUT` | `48` | 单次任务超时时间，单位小时，`0` 表示不限制 |
| `OPENSYNC_COPY_CONCURRENCY` | `5` | 单个任务的复制并发数，范围 `1` 到 `100` |
| `OPENSYNC_SCAN_CONCURRENCY` | `8` | 单个任务的扫描并发数，范围 `1` 到 `20` |
| `OPENSYNC_REALTIME_FINISHED_ITEMS` | `1000` | 实时任务页保留的已完成明细数量 |
| `OPENSYNC_MAX_RETRIES` | `0` | 单个复制项失败后的最大自动重试次数，`0` 表示不自动重试 |

如果需要使用配置文件，可以创建或通过系统设置页生成 `data/config.ini`：

```ini
[opensync]
port=8023
expires=7
log_level=1
console_level=2
log_save=7
task_save=30
task_timeout=48
copy_concurrency=5
scan_concurrency=8
realtime_finished_items=1000
max_retries=0
```

系统设置页可在线调整其中的运行配置。端口、日志等级等启动期配置仍建议通过环境变量或配置文件维护。

## 本地构建镜像

```bash
docker build -t opensync .
docker run -d \
  --name opensync \
  --restart unless-stopped \
  -p 8023:8023 \
  -v opensync-data:/app/data \
  -e OPENSYNC_PORT=8023 \
  -e GIN_MODE=release \
  opensync
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

## 开发检查

```bash
cd frontend
npm run build

cd ../backend
go test ./...
```

## 发布和 Docker 镜像

发布 GitHub Release 后，GitHub Actions 会自动构建并推送 Docker 镜像到 GitHub Container Registry。

发布 `v1.4.0` 后会生成：

- `ghcr.io/chenbin3625/opensync:1.4.0`
- `ghcr.io/chenbin3625/opensync:1.4`
- `ghcr.io/chenbin3625/opensync:latest`
- `ghcr.io/chenbin3625/opensync:sha-<commit>`

当前自动发布的镜像平台为 `linux/amd64`，适合常见 x86_64 飞牛系统、群晖、NAS 和服务器设备。

## 注意事项

- 不要提交或公开 `backend/data`、Docker 挂载的 `data/` 目录或任何包含 AList / OpenList Token 的文件。
- `data/secret.key` 会影响登录 Cookie 和敏感信息加解密，部署后应通过持久化目录保留。
- 如果误分享了运行数据目录，请及时更换 AList / OpenList Token。
- 文件大小过滤和运行配置属于 1.4.0 新增能力，旧版本升级后会自动补齐数据库字段。
