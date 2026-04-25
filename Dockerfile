# Build target: "server" (default) or "skill-server"
ARG BUILD_TARGET=server

# ── Builder ──────────────────────────────────────────────────────────────────
FROM golang:1.21-bullseye AS builder

WORKDIR /app

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum* ./
RUN go mod download

COPY cmd/ ./cmd/
COPY internal/ ./internal/

# Build both binaries (pure Go, no CGO needed)
RUN CGO_ENABLED=0 GOOS=linux go build -o kapi-server ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux go build -o skill-server ./cmd/skill-server

# ── Runtime ───────────────────────────────────────────────────────────────────
FROM debian:bullseye-slim

ARG BUILD_TARGET=server

WORKDIR /app

RUN apt-get update && apt-get install -y ca-certificates wget && rm -rf /var/lib/apt/lists/*

# Copy both binaries; CMD选用哪个由 BUILD_TARGET 决定
COPY --from=builder /app/kapi-server .
COPY --from=builder /app/skill-server .
COPY web-client ./web-client

RUN mkdir -p logs

EXPOSE 8080 8090

ENV BUILD_TARGET=${BUILD_TARGET}

HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:${PORT:-8080}/health || exit 1

CMD if [ "$BUILD_TARGET" = "skill-server" ]; then ./skill-server; else ./kapi-server; fi
