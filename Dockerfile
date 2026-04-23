# Build stage
FROM golang:1.21-bullseye AS builder

WORKDIR /app

# Install build dependencies (gcc needed for CGO plugins)
RUN apt-get update && apt-get install -y --no-install-recommends gcc && rm -rf /var/lib/apt/lists/*

# Copy go mod files
COPY go.mod go.sum* ./

# Download dependencies
RUN go mod download

# Copy source code
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY skills/ ./skills/

# Build plugins first
RUN mkdir -p bin/skills && \
    find ./skills/financial -name "*.go" -type f -exec sh -c ' \
        dir=$(dirname "$1"); \
        name=$(basename "$dir"); \
        json="$dir/$name.json"; \
        if [ -f "$json" ]; then \
            id=$(grep -o "\"id\"[[:space:]]*:[[:space:]]*\"[^\"]*\"" "$json" | sed "s/.*\"\([^\"]*\)\".*/\1/" | head -1); \
            [ -z "$id" ] && id="$name"; \
            go build -buildmode=plugin -o "bin/skills/${id}.so" "$1" || true; \
        fi \
    ' sh {} \;

# Build main application (CGO_ENABLED=1 for plugin support)
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -a -installsuffix cgo -o kapi-server ./cmd/server

# Runtime stage
FROM debian:bullseye-slim

WORKDIR /app

# Install ca-certificates and wget for HTTPS requests
RUN apt-get update && apt-get install -y ca-certificates wget && rm -rf /var/lib/apt/lists/*

# Copy the binary and plugins from builder
COPY --from=builder /app/kapi-server .
COPY --from=builder /app/bin/skills ./skills

# Make .so files executable
RUN chmod +x ./skills/*.so

# Copy web-client
COPY web-client ./web-client

# Create logs directory
RUN mkdir -p logs

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Run the application
CMD ["./kapi-server"]
