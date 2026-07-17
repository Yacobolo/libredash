# syntax=docker/dockerfile:1.7@sha256:a57df69d0ea827fb7266491f2813635de6f17269be881f696fbfdf2d83dda33e

FROM node:24-bookworm@sha256:392e1e23f34da768d8d1f4e502b64f200d3be3465934d4b7930f57d7e2fc1989 AS node

FROM golang:1.25-bookworm@sha256:a9c020ee3d1508c7be5435c262434e3d3fc1d0e76a11afeb9ddae7d60bc86aa4 AS sourcegen
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
    go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0 generate && \
    go run ./internal/tools/configgen && \
    go run github.com/Yacobolo/toolbelt/apigen/cmd/apigen@v0.5.3 typespec-compile -manifest api/apigen.yaml -target libredash-v1 && \
    go run github.com/Yacobolo/toolbelt/apigen/cmd/apigen@v0.5.3 all -manifest api/apigen.yaml -target libredash-v1 && \
    go run github.com/Yacobolo/toolbelt/apigen/cmd/apigen@v0.5.3 typespec-compile -manifest api/apigen.yaml -target ui-signals && \
    go run github.com/Yacobolo/toolbelt/apigen/cmd/apigen@v0.5.3 all -manifest api/apigen.yaml -target ui-signals && \
    go run ./internal/tools/uisignalspostprocess && \
    go run ./cmd/libredash schema export --format json-schema --out schemas/json

FROM oven/bun:1.3.7@sha256:6cd5f00020e48b77a253bc8249f6b6dd3d92b3c04c2607f1f5a6d7dbf0a6fca3 AS web
WORKDIR /src

COPY package.json bun.lock tsconfig.json ./
COPY scripts ./scripts
COPY static ./static
COPY web ./web
COPY --from=sourcegen /src/web/generated ./web/generated

RUN bun install --frozen-lockfile --no-cache
RUN bun run build

FROM golang:1.25-bookworm@sha256:a9c020ee3d1508c7be5435c262434e3d3fc1d0e76a11afeb9ddae7d60bc86aa4 AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=sourcegen /src/api/gen ./api/gen
COPY --from=sourcegen /src/internal/api/gen ./internal/api/gen
COPY --from=sourcegen /src/internal/cli/gen ./internal/cli/gen
COPY --from=sourcegen /src/internal/platform/db/db.go ./internal/platform/db/db.go
COPY --from=sourcegen /src/internal/platform/db/models.go ./internal/platform/db/models.go
COPY --from=sourcegen /src/internal/platform/db/*.sql.go ./internal/platform/db/
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
    LIBREDASH_MANAGED_DATA_DIR=/var/lib/libredash/managed-data \
    LIBREDASH_PRODUCTION=1

EXPOSE 8080
VOLUME ["/var/lib/libredash"]
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 CMD ["libredash", "healthcheck"]

ENTRYPOINT ["libredash"]
CMD ["serve", "--production"]
