# OpenSync

OpenSync 是为飞牛 fnOS / 飞牛 NAS 准备的 AList / OpenList 自动同步工具，可以作为飞牛 NAS 下的群晖 Cloud Sync 平替方案。

它通过 AList / OpenList 统一接入本地目录、网盘、对象存储、WebDAV 等存储端，再用可视化任务完成自动备份、归档和迁移。适合把照片库、影音库、下载目录、文档目录同步到网盘、对象存储或另一台存储设备。

![OpenSync 任务总览](https://raw.githubusercontent.com/chenbin3625/OpenSync/main/docs/images/tasks-overview.png)

## 适合场景

- 飞牛 fnOS / 飞牛 NAS 自动同步和备份。
- 替代群晖 Cloud Sync，完成跨网盘、跨对象存储、跨 WebDAV 的同步。
- 家庭 NAS 照片库、影音库、下载目录、文档目录定时备份。
- 多个 AList / OpenList 引擎统一管理。
- 同步任务实时进度、失败原因和历史记录追踪。
- 任务完成或失败后通过 Webhook、Server 酱、钉钉、企业微信、飞书等渠道通知。

## 快速部署

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
    environment:
      OPENSYNC_PORT: 8023
      GIN_MODE: release
```

## 镜像标签

- `chenbin3625/opensync:latest`
- `chenbin3625/opensync:1.4.0`
- `chenbin3625/opensync:1.4`

镜像支持以下平台：

- `linux/amd64`
- `linux/arm64`
- `linux/arm/v7`

适合常见 x86_64、ARM64 和 ARMv7 架构的飞牛系统、NAS 和服务器设备。

## 主要功能

- 多 AList / OpenList 引擎管理。
- 仅新增、全同步、移动模式。
- 手动执行、间隔执行、Cron 定时执行。
- 多源目录、多目标目录同步。
- 文件大小过滤和 Gitignore 风格排除规则。
- 实时扫描进度、传输速度、剩余大小和任务明细。
- 运行中任务暂停、历史任务继续执行、失败项单独重试。
- Webhook、Server 酱、钉钉、企业微信、飞书通知。
- 登录、修改密码、深色模式、中英文语言。
- SQLite 本地数据存储。

## 数据持久化

请持久化 `/app/data`。该目录包含数据库、密钥、配置和日志。

不要公开 `data/` 目录或任何包含 AList / OpenList Token 的文件。`data/secret.key` 会影响登录 Cookie 和敏感信息加解密，部署后应保留。

## 项目地址

GitHub: https://github.com/chenbin3625/OpenSync
