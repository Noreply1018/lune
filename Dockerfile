# ── Stage 1: Build frontend ──────────────────────────────
FROM node:22-slim AS frontend

WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build
# Output: /web/../internal/site/dist/ → but we use outDir relative,
# so let's just copy the result later from the known location.
# vite.config.ts outputs to ../internal/site/dist which is /internal/site/dist

# ── Stage 2: Build Go binary ────────────────────────────
FROM golang:1.24 AS builder

WORKDIR /app
COPY go.mod go.sum ./
ENV GOPROXY=https://goproxy.cn,direct
RUN go mod download

COPY . .
# Inject the frontend build output into the embed directory
COPY --from=frontend /internal/site/dist /app/internal/site/dist
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /lune ./cmd/lune

# ── Stage 3: Runtime ────────────────────────────────────
FROM debian:bookworm-slim

WORKDIR /app
RUN mkdir -p /app/data /app/configs

COPY --from=builder /lune /usr/local/bin/lune
COPY configs/config.example.json /app/configs/config.example.json

EXPOSE 7788

ENV LUNE_CONFIG=/app/configs/config.json

CMD ["lune"]
