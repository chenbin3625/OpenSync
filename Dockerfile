# syntax=docker/dockerfile:1.7

# Stage 1: Build frontend
FROM --platform=$BUILDPLATFORM node:20-alpine AS frontend-builder
WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json* ./
RUN npm ci
COPY frontend/ .
RUN npm run build

# Stage 2: Build backend
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS backend-builder
RUN apk add --no-cache git
WORKDIR /app
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ .
COPY docker-entrypoint.go /tmp/docker-entrypoint.go
# Copy frontend build output into Go embed directory
COPY --from=frontend-builder /app/backend/cmd/server/web/ ./cmd/server/web/
ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT
RUN set -eux; \
    target_os="${TARGETOS:-linux}"; \
    target_arch="${TARGETARCH:-$(go env GOARCH)}"; \
    if [ "$target_arch" = "arm" ]; then \
      target_variant="${TARGETVARIANT#v}"; \
      export GOARM="${target_variant:-7}"; \
    fi; \
    CGO_ENABLED=0 GOOS="$target_os" GOARCH="$target_arch" \
      go build -trimpath -ldflags="-s -w" -o opensync ./cmd/server/; \
    CGO_ENABLED=0 GOOS="$target_os" GOARCH="$target_arch" \
      go build -trimpath -ldflags="-s -w" -o docker-entrypoint /tmp/docker-entrypoint.go

# Stage 3: Runtime data
FROM --platform=$BUILDPLATFORM alpine:3.20 AS runtime-deps
RUN apk add --no-cache ca-certificates tzdata

# Stage 4: Runtime
FROM scratch
WORKDIR /tmp
WORKDIR /app
COPY --from=runtime-deps /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=runtime-deps /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=backend-builder /app/opensync .
COPY --from=backend-builder /app/docker-entrypoint .
ENV SSL_CERT_FILE=/etc/ssl/certs/ca-certificates.crt
ENV TZ=Asia/Shanghai
ENV PUID=1000
ENV PGID=1000
ENV OPENSYNC_BIND=0.0.0.0
ENV OPENSYNC_PORT=8023
ENV GIN_MODE=release
EXPOSE 8023
VOLUME ["/app/data"]
ENTRYPOINT ["./docker-entrypoint"]
CMD ["./opensync"]
