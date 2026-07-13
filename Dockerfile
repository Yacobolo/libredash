# syntax=docker/dockerfile:1.7

FROM node:24-bookworm AS node

FROM golang:1.25-bookworm AS sourcegen
WORKDIR /src

COPY --from=node /usr/local/bin/node /usr/local/bin/node
COPY --from=node /usr/local/lib/node_modules /usr/local/lib/node_modules
RUN ln -sf ../lib/node_modules/npm/bin/npm-cli.js /usr/local/bin/npm && \
    ln -sf ../lib/node_modules/npm/bin/npx-cli.js /usr/local/bin/npx

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    go run github.com/Yacobolo/toolbelt/apigen/cmd/apigen@v0.4.0 typespec-compile -manifest api/apigen.yaml -target libredash-v1 && \
    go run github.com/Yacobolo/toolbelt/apigen/cmd/apigen@v0.4.0 all -manifest api/apigen.yaml -target libredash-v1 && \
    go run ./internal/tools/apigenpostprocess && \
    go run ./cmd/libredash schema export --format json-schema --out schemas/json && \
    go run ./internal/tools/uisignalsgen

FROM oven/bun:1.3.7 AS web
WORKDIR /src

COPY package.json bun.lock tsconfig.json ./
COPY scripts ./scripts
COPY static ./static
COPY web ./web
COPY --from=sourcegen /src/web/generated ./web/generated

RUN bun install --frozen-lockfile --no-cache
RUN bun run build

FROM golang:1.25-bookworm AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=sourcegen /src/api/gen ./api/gen
COPY --from=sourcegen /src/internal/api/gen ./internal/api/gen
COPY --from=sourcegen /src/internal/cli/gen ./internal/cli/gen
COPY --from=sourcegen /src/schemas ./schemas
COPY --from=sourcegen /src/web/generated ./web/generated
COPY --from=web /src/static ./static

RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o /out/libredash ./cmd/libredash

FROM debian:bookworm-slim AS runtime

RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates libstdc++6 tzdata && \
    rm -rf /var/lib/apt/lists/*

RUN groupadd --system libredash && \
    useradd --system --gid libredash --home-dir /var/lib/libredash --shell /usr/sbin/nologin libredash

WORKDIR /app

COPY --from=build /out/libredash /usr/local/bin/libredash
COPY --from=web /src/static ./static
COPY --from=build /src/schemas ./schemas
COPY dashboards ./dashboards

RUN mkdir -p /var/lib/libredash && \
    chown -R libredash:libredash /var/lib/libredash /app

USER libredash

ENV LIBREDASH_ADDR=:8080 \
    LIBREDASH_HOME=/var/lib/libredash \
    LIBREDASH_PRODUCTION=1

EXPOSE 8080
VOLUME ["/var/lib/libredash"]
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 CMD ["libredash", "healthcheck"]

ENTRYPOINT ["libredash"]
CMD ["serve", "--production"]
