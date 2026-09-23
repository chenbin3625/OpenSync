# Contributing to OpenSync

Thanks for helping improve OpenSync. This project focuses on practical AList / OpenList sync automation for fnOS, NAS, and Docker users, so clear reproduction steps and real deployment context are especially valuable.

## Where to Start

- Use GitHub Issues for reproducible bugs.
- Use GitHub Discussions for usage help, deployment cases, feature ideas, and broader design conversations.
- Search existing issues and discussions before opening a new one.
- Keep reports free of tokens, private paths, account names, and hostnames.

## Development Setup

Start the backend:

```bash
cd backend
go run ./cmd/server
```

Start the frontend dev server:

```bash
cd frontend
npm install
npm run dev
```

The frontend dev server runs at:

```text
http://127.0.0.1:3000/
```

It proxies `/svr` requests to:

```text
http://localhost:8023
```

## Checks Before a Pull Request

Run the relevant checks before opening a PR:

```bash
cd frontend
npm run test
npm run build

cd ../backend
go test ./...
go vet ./...
```

For Docker or startup changes, also test a local image build:

```bash
docker build -t opensync .
```

## Pull Request Guidelines

- Keep the PR focused on one bug fix, feature, or documentation change.
- Update README, Docker Hub docs, or screenshots when user-facing behavior changes.
- Add or update tests for behavior changes when practical.
- Do not include secrets, tokens, private paths, local database files, or runtime `data/` contents.
- Explain the scenario you tested and include screenshots for UI changes.

## Documentation and Promotion Assets

- The main `README.md` is Chinese-first and is the primary entry point; it points fnOS users to [OpenSync-fnOS](https://github.com/chenbin3625/OpenSync-fnOS) and its native fpk build.
- `README.en.md` is the English documentation.
- `.dockerhub/README.md` is the Docker Hub long description source.
- `docs/demo/opensync-demo.gif` is the 60-second promotional demo asset.

---

# 贡献指南

感谢你帮助改进 OpenSync。这个项目聚焦飞牛 fnOS、NAS、Docker 用户在 AList / OpenList 上的自动同步需求，所以清晰的复现步骤和真实部署背景很重要。

## 从哪里开始

- 可复现问题请提交 GitHub Issue。
- 使用求助、部署案例、功能想法和较开放的设计讨论请发 GitHub Discussions。
- 提交前请先搜索现有 Issue 和 Discussion。
- 不要在内容中包含令牌、私有路径、账号名和主机名。

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

## 提交 PR 前检查

提交 PR 前请尽量运行相关检查：

```bash
cd frontend
npm run test
npm run build

cd ../backend
go test ./...
go vet ./...
```

涉及 Docker 或启动流程时，也建议测试本地镜像构建：

```bash
docker build -t opensync .
```

## Pull Request 要求

- 每个 PR 尽量聚焦一个问题修复、功能或文档改动。
- 用户可见行为变化时，同步更新 README、Docker Hub 文档或截图。
- 行为变化较明显时，尽量补充或更新测试。
- 不要提交密钥、令牌、私有路径、本地数据库文件或运行时 `data/` 内容。
- UI 改动请说明测试场景，并尽量附截图。

## 文档和推广素材

- 主 `README.md` 以中文优先，是主要入口，并在顶部引导飞牛用户使用 [OpenSync-fnOS](https://github.com/chenbin3625/OpenSync-fnOS) 原生 fpk 版。
- `README.en.md` 是英文文档。
- `.dockerhub/README.md` 是 Docker Hub 长描述来源。
- `docs/demo/opensync-demo.gif` 是 60 秒推广演示素材。
