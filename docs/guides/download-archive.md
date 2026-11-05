# 下载目录自动归档到另一台 NAS

这套配置适合把下载完成的文件从本地下载目录定时移动到另一台 NAS、WebDAV 或冷存储目录，减少工作盘占用。第一次配置建议先用「仅新增」验证路径和权限，再切换到「移动」模式。

## 30 秒安装

```bash
mkdir -p opensync && cd opensync && curl -fsSLO https://raw.githubusercontent.com/chenbin3625/OpenSync/main/docker-compose.yml && docker compose up -d
```

打开 `http://你的设备IP:8023/`，创建管理员账号并保存恢复密钥。飞牛用户也可以直接安装 [OpenSync-fnOS 原生 fpk](https://github.com/chenbin3625/OpenSync-fnOS/releases)。

## 配置流程

```mermaid
flowchart LR
  A[下载目录] --> B[OpenSync 任务]
  B --> C[另一台 NAS / WebDAV]
  C --> D[目标端确认]
  D --> E[源端释放空间]
```

1. 在 OpenList / AList 中添加下载目录和另一台 NAS、WebDAV 或对象存储。
2. 在 OpenSync 中先创建「仅新增」任务，筛选已完成文件或指定扩展名，手动运行并确认目标端内容。
3. 确认目标端路径、权限和剩余空间都正确后，把任务切换为「移动」模式，并设置每天或按分钟执行。
4. 需要保留源端一段时间时，继续使用「仅新增」；不要把同一底层数据的别名、重复挂载或符号链接同时当作移动源和目标。

## 失败恢复

任务失败时先暂停计划，在「历史任务」查看失败文件和目标端响应。修复权限、网络或空间问题后，只重试未完成项；未确认成功的文件不要手动删除源端副本。第一次运行移动任务时，建议只选一个小目录作为验收样本。

## 发布入口

从 [OpenSync Releases](https://github.com/chenbin3625/OpenSync/releases) 下载二进制或查看对应版本的升级说明；fnOS 用户使用 [OpenSync-fnOS Releases](https://github.com/chenbin3625/OpenSync-fnOS/releases)。

不要在公开 Issue、截图或日志中暴露存储端 Token、账号和私有路径。
