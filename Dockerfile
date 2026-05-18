# syntax=docker/dockerfile:1.6
#
# Multi-stage build for myserver.
# Builder: Tailwind CLI standalone (no Node.js) + templ + Go.
# Runtime: alpine + su-exec + bash + docker-cli (for script wrappers).
#
# The build is fully reproducible from the repo: no npm, no node_modules.

FROM golang:1.25-alpine AS builder

WORKDIR /build

# Install build tooling: curl for tailwind binary, ca-certificates, wget.
RUN apk add --no-cache curl ca-certificates

# Install Tailwind CSS standalone CLI (architecture-aware).
# IMPORTANT: pinned to v3.4.17 — input.css uses `@layer base` with `@apply`
# which Tailwind v4 does not support without the `@reference` directive.
# When migrating to v4 we must rewrite input.css per the v4 upgrade guide.
ARG TAILWIND_VERSION=v3.4.17
RUN ARCH=$(case "$(uname -m)" in \
        aarch64) echo "linux-arm64" ;; \
        x86_64) echo "linux-x64" ;; \
        *) echo "linux-x64" ;; \
    esac) && \
    curl -sLo /usr/local/bin/tailwindcss "https://github.com/tailwindlabs/tailwindcss/releases/download/${TAILWIND_VERSION}/tailwindcss-${ARCH}" && \
    chmod +x /usr/local/bin/tailwindcss

# Install templ compiler. Version must match the templ runtime pinned in
# go.mod, otherwise the generator emits calls (e.g. templ.ResolveAttributeValue)
# that don't exist in the runtime and the Go build fails.
ARG TEMPL_VERSION=v0.3.1001
RUN go install github.com/a-h/templ/cmd/templ@${TEMPL_VERSION} && \
    cp "$(go env GOPATH)/bin/templ" /usr/local/bin/templ

# Cache module deps.
COPY go.mod go.sum ./
RUN go mod download

# Copy source.
COPY . .

# Generate templ files and Tailwind CSS, then build the static binary.
RUN templ generate && \
    tailwindcss -i web/tailwind/input.css -o web/static/css/main.css -c tailwind.config.js --minify && \
    CGO_ENABLED=0 go build -ldflags="-s -w" -o myserver ./cmd/myserver/

# ---------------------------------------------------------------------------
# Runtime stage
# ---------------------------------------------------------------------------
FROM alpine:3.21

# - su-exec: drop privileges from root to myserver user
# - bash: scripts feature requires real bash
# - docker-cli: scripts can call `docker restart`, `docker ps`, etc
# - wget: docker-compose healthcheck
# - tini: clean signal handling for the script subprocesses
# - tzdata: TZ env var support
RUN apk add --no-cache su-exec bash docker-cli wget tini tzdata && \
    addgroup -g 1000 myserver && \
    adduser -u 1000 -G myserver -D -H myserver && \
    mkdir -p /app/config /app/scripts /app/data && \
    chown -R myserver:myserver /app

COPY --from=builder --chown=myserver:myserver /build/myserver /app/myserver
COPY --from=builder --chown=myserver:myserver /build/web/static /app/web/static
COPY --from=builder --chown=myserver:myserver /build/internal/config/skeleton /app/skeleton
COPY --chown=myserver:myserver docker-entrypoint.sh /app/

RUN chmod +x /app/docker-entrypoint.sh /app/myserver

WORKDIR /app
EXPOSE 3000

# OCI labels — Coolify and registries surface these in the UI.
LABEL org.opencontainers.image.title="MyServer" \
      org.opencontainers.image.description="Self-hosted dashboard for thotenn.com (Go rewrite)" \
      org.opencontainers.image.licenses="GPL-3.0" \
      org.opencontainers.image.source="https://github.com/thotenn/myserver"

ENTRYPOINT ["/sbin/tini", "--", "/app/docker-entrypoint.sh"]
CMD ["/app/myserver"]