# -- Stage 1: Build frontend --
FROM --platform=$BUILDPLATFORM node:22-slim AS frontend

WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# -- Stage 2: Build Go binary --
FROM --platform=$BUILDPLATFORM golang:1.25 AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=frontend /internal/site/dist /app/internal/site/dist
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build -o /lune ./cmd/lune

# -- Stage 3: Runtime --
FROM debian:bookworm-slim

WORKDIR /app
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && mkdir -p /app/data

COPY --from=builder /lune /usr/local/bin/lune

EXPOSE 7788

ENV LUNE_PORT=7788
ENV LUNE_DATA_DIR=/app/data

CMD ["lune"]
