ARG CPA_VERSION=v7.0.2
ARG CPA_COMMIT=1fca942b9c2c5bbdf78334eb4744a098983a05e9
ARG CPA_PATCH_VERSION=v7.0.2-lune.1

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
FROM --platform=$BUILDPLATFORM golang:1.26 AS cpa-builder

ARG TARGETOS
ARG TARGETARCH
ARG CPA_VERSION=v7.0.2
ARG CPA_COMMIT=1fca942b9c2c5bbdf78334eb4744a098983a05e9
ARG CPA_PATCH_VERSION=v7.0.2-lune.1
ARG LUNE_BUILD_DATE=unknown
ARG GOPROXY=https://proxy.golang.org,direct

WORKDIR /src
RUN git clone --depth 1 --branch "${CPA_VERSION}" https://github.com/router-for-me/CLIProxyAPI.git . \
    && test "$(git rev-parse HEAD)" = "${CPA_COMMIT}"
COPY third_party/cliproxyapi/v7.0.2-lune-provider-pinning.patch /tmp/lune-provider-pinning.patch
RUN git apply /tmp/lune-provider-pinning.patch \
    && GOPROXY=${GOPROXY} CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build \
    -ldflags "-X main.Version=${CPA_PATCH_VERSION} -X main.Commit=${CPA_COMMIT} -X main.BuildDate=${LUNE_BUILD_DATE}" \
    -o /CLIProxyAPI/CLIProxyAPI ./cmd/server

# -- Stage 4: Runtime --
FROM debian:bookworm-slim
ARG CPA_VERSION=v7.0.2
ARG CPA_PATCH_VERSION=v7.0.2-lune.1

WORKDIR /app
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl \
    && rm -rf /var/lib/apt/lists/* \
    && mkdir -p /app/data/cpa-auth /app/data/tmp /CLIProxyAPI

COPY --from=builder /lune /usr/local/bin/lune
COPY --from=cpa-builder /CLIProxyAPI/CLIProxyAPI /CLIProxyAPI/CLIProxyAPI
COPY docker/entrypoint.sh /usr/local/bin/lune-entrypoint
RUN chmod +x /usr/local/bin/lune-entrypoint /CLIProxyAPI/CLIProxyAPI

EXPOSE 7788

ENV LUNE_PORT=7788
ENV LUNE_DATA_DIR=/app/data
ENV LUNE_CPA_AUTH_DIR=/app/data/cpa-auth
ENV LUNE_GATEWAY_TMP_DIR=/app/data/tmp
ENV LUNE_EMBEDDED_CPA_VERSION=${CPA_PATCH_VERSION}

ENTRYPOINT ["lune-entrypoint"]
CMD ["up"]
