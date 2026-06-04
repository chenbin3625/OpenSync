# Stage 1: Build frontend
FROM node:20-alpine AS frontend-builder
WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json* ./
RUN npm install --frozen-lockfile 2>/dev/null || npm install
COPY frontend/ .
RUN npm run build

# Stage 2: Build backend
FROM golang:1.26-alpine AS backend-builder
RUN apk add --no-cache git
WORKDIR /app
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ .
# Copy frontend build output into Go embed directory
COPY --from=frontend-builder /app/backend/cmd/server/web/ ./cmd/server/web/
RUN go build -o opensync ./cmd/server/

# Stage 3: Runtime
FROM alpine:3.20
WORKDIR /app
COPY --from=backend-builder /app/opensync .
RUN mkdir -p data/log
ENV OPENSYNC_PORT=8023
ENV GIN_MODE=release
EXPOSE 8023
VOLUME ["/app/data"]
CMD ["./opensync"]
