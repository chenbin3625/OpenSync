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
      go build -trimpath -ldflags="-s -w" -o opensync ./cmd/server/

# Stage 3: Runtime
FROM alpine:3.20
WORKDIR /app
COPY --from=backend-builder /app/opensync .
ENV OPENSYNC_PORT=8023
ENV GIN_MODE=release
EXPOSE 8023
VOLUME ["/app/data"]
CMD ["./opensync"]
