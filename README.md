# OpenSync

OpenSync is an AList automation tool for scheduling file synchronization jobs, tracking task progress, and sending completion notifications.

## Features

- Manage multiple AList engines
- Create manual or scheduled synchronization jobs
- Support add-only sync, full sync, and move mode
- Track live progress, transfer speed, file counts, failures, and task history
- View task details in nested drawers without leaving the job page
- Configure notification channels including webhook, ServerChan, DingTalk, WeCom, and Lark
- Built-in login, password management, dark mode, and SQLite storage
- Docker deployment with persistent runtime data

## Docker

Pull the latest image:

```bash
docker pull ghcr.io/chenbin3625/opensync:latest
```

Run OpenSync:

```bash
docker run -d \
  --name opensync \
  -p 8023:8023 \
  -v opensync-data:/app/data \
  ghcr.io/chenbin3625/opensync:latest
```

Open `http://127.0.0.1:8023/`.

On first launch, the initial admin password is printed in the container logs:

```bash
docker logs opensync
```

## Build Locally

```bash
docker build -t opensync .
docker run -d \
  --name opensync \
  -p 8023:8023 \
  -v opensync-data:/app/data \
  opensync
```

## Local Development

Start the backend:

```bash
cd backend
go run ./cmd/server
```

Start the frontend dev server in another terminal:

```bash
cd frontend
npm install
npm run dev
```

The frontend runs on `http://127.0.0.1:3000/` and proxies `/svr` API calls to `http://localhost:8023`.

## Production Build Without Docker

Build the frontend first. The Vite config writes static files into the Go embed directory.

```bash
cd frontend
npm install
npm run build
```

Then build and run the backend:

```bash
cd ../backend
go build -o opensync ./cmd/server
./opensync
```

## Configuration

Runtime data is stored under `backend/data` during local development and `/app/data` in Docker. This includes the SQLite database, generated secret key, language preference, and logs.

Environment variables are used when `data/config.ini` is not present:

| Variable | Default | Description |
| --- | --- | --- |
| `TAO_PORT` | `8023` | HTTP server port |
| `TAO_EXPIRES` | `2` | Login expiration in days |
| `TAO_LOG_LEVEL` | `1` | File log level |
| `TAO_CONSOLE_LEVEL` | `2` | Console log level |
| `TAO_LOG_SAVE` | `7` | Log retention in days |
| `TAO_TASK_SAVE` | `0` | Task retention, `0` keeps all tasks |
| `TAO_TASK_TIMEOUT` | `72` | Task timeout in hours |

## Docker Release Automation

Publishing a GitHub Release or pushing a semantic version tag triggers the Docker workflow and pushes a multi-architecture image to GitHub Container Registry.

Use semantic version tags such as `v1.2.3`. A published release creates:

- `ghcr.io/chenbin3625/opensync:1.2.3`
- `ghcr.io/chenbin3625/opensync:1.2`
- `ghcr.io/chenbin3625/opensync:latest` for non-prerelease versions
- `ghcr.io/chenbin3625/opensync:sha-<commit>`

The workflow can also be run manually from the GitHub Actions tab.

## Development Checks

```bash
cd frontend
npm run build

cd ../backend
go test ./...
```

`npm run lint` currently reports strict lint issues. Use frontend build and backend tests as the baseline checks until lint cleanup is complete.

## Security Notes

- Do not commit runtime files from `backend/data`.
- Keep the generated `secret.key` stable for a deployed instance by using a persistent data volume.
- Rotate AList tokens if a local runtime directory is ever shared accidentally.
