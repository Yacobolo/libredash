# syntax=docker/dockerfile:1.7@sha256:a57df69d0ea827fb7266491f2813635de6f17269be881f696fbfdf2d83dda33e

FROM node:26-bookworm@sha256:e2cd0ff87e2597f66fab50710216e2a08ad2f09bae0ca78f6b31e8c5f1a811a0 AS node

FROM golang:1.26-bookworm@sha256:18aedc16aa19b3fd7ded7245fc14b109e054d65d22ed53c355c899582bbb2113 AS sourcegen
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
    go run ./internal/tools/configgen && \
    go run github.com/Yacobolo/toolbelt/apigen/cmd/apigen@v0.4.0 typespec-compile -manifest api/apigen.yaml -target libredash-v1 && \
    go run github.com/Yacobolo/toolbelt/apigen/cmd/apigen@v0.4.0 all -manifest api/apigen.yaml -target libredash-v1 && \
    go run ./internal/tools/apigenpostprocess && \
    go run github.com/Yacobolo/toolbelt/apigen/cmd/apigen@v0.4.0 typespec-compile -manifest api/apigen.yaml -target ui-signals && \
    go run github.com/Yacobolo/toolbelt/apigen/cmd/apigen@v0.4.0 all -manifest api/apigen.yaml -target ui-signals && \
    go run ./internal/tools/uisignalspostprocess && \
    go run ./cmd/libredash schema export --format json-schema --out schemas/json

FROM oven/bun:1.3.14@sha256:e10577f0db68676a7024391c6e5cb4b879ebd17188ab750cf10024a6d700e5c4 AS web
WORKDIR /src

COPY package.json bun.lock tsconfig.json ./
COPY scripts ./scripts
COPY static ./static
COPY web ./web
COPY --from=sourcegen /src/web/generated ./web/generated

RUN bun install --frozen-lockfile --no-cache
RUN bun run build

FROM golang:1.26-bookworm@sha256:18aedc16aa19b3fd7ded7245fc14b109e054d65d22ed53c355c899582bbb2113 AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=sourcegen /src/api/gen ./api/gen
COPY --from=sourcegen /src/internal/api/gen ./internal/api/gen
COPY --from=sourcegen /src/internal/cli/gen ./internal/cli/gen
COPY --from=sourcegen /src/internal/ui/signals/models.gen.go ./internal/ui/signals/models.gen.go
COPY --from=sourcegen /src/schemas ./schemas
COPY --from=sourcegen /src/web/generated ./web/generated
COPY --from=web /src/static ./static

RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o /out/libredash ./cmd/libredash

FROM debian:bookworm-slim@sha256:60eac759739651111db372c07be67863818726f754804b8707c90979bda511df AS runtime

ARG BUILD_VERSION=dev
ARG BUILD_REVISION=unknown

LABEL org.opencontainers.image.title="LibreDash" \
      org.opencontainers.image.description="LibreDash business intelligence server" \
      org.opencontainers.image.source="https://github.com/Yacobolo/libredash" \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.version="$BUILD_VERSION" \
      org.opencontainers.image.revision="$BUILD_REVISION"

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
