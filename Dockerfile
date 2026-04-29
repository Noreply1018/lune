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
ARG LUNE_VERSION=dev
ARG LUNE_COMMIT=unknown
ARG LUNE_BUILD_DATE=unknown

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=frontend /internal/site/dist /app/internal/site/dist
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build \
    -ldflags "-X main.version=${LUNE_VERSION} -X main.commit=${LUNE_COMMIT} -X main.date=${LUNE_BUILD_DATE}" \
    -o /lune ./cmd/lune

# -- Stage 3: CPA binary --
ARG CPA_VERSION=v6.9.41
FROM eceasy/cli-proxy-api:v6.9.41@sha256:27a8090de418fd5ef96fae91ba6ba8579874806d573c5de3f8d13a1a4fe5ee91 AS cpa

# -- Stage 4: Runtime --
FROM debian:bookworm-slim
ARG CPA_VERSION=v6.9.41

WORKDIR /app
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && mkdir -p /app/data/cpa-auth /app/data/tmp /CLIProxyAPI

COPY --from=builder /lune /usr/local/bin/lune
COPY --from=cpa /CLIProxyAPI/CLIProxyAPI /CLIProxyAPI/CLIProxyAPI
COPY docker/entrypoint.sh /usr/local/bin/lune-entrypoint
RUN chmod +x /usr/local/bin/lune-entrypoint /CLIProxyAPI/CLIProxyAPI

EXPOSE 7788

ENV LUNE_PORT=7788
ENV LUNE_DATA_DIR=/app/data
ENV LUNE_CPA_AUTH_DIR=/app/data/cpa-auth
ENV LUNE_GATEWAY_TMP_DIR=/app/data/tmp
ENV LUNE_EMBEDDED_CPA_VERSION=${CPA_VERSION}

ENTRYPOINT ["lune-entrypoint"]
CMD ["up"]
