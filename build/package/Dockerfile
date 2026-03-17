# Multi-stage build for O3K - Multi-architecture support
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder

# Build arguments for cross-compilation
ARG TARGETOS
ARG TARGETARCH

# Install build dependencies
RUN apk add --no-cache git make

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary for target architecture
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -ldflags="-w -s" -o o3k ./cmd/o3k

# Build the migration tool for target architecture
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -ldflags="-w -s" -o o3k-migrate ./cmd/o3k-migrate

# Runtime stage - automatically uses target platform
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache \
    ca-certificates \
    curl \
    tzdata \
    iproute2 \
    iptables \
    ipset

# Create non-root user (but will run as root in privileged mode for networking)
RUN addgroup -g 1000 o3k && \
    adduser -D -u 1000 -G o3k o3k

# Set working directory
WORKDIR /app

# Copy binaries from builder
COPY --from=builder /build/o3k /app/o3k
COPY --from=builder /build/o3k-migrate /app/o3k-migrate

# Copy migrations
COPY migrations /app/migrations

# Copy default config
COPY config/o3k.yaml /app/config/o3k.yaml

# Create storage directories for local mode
RUN mkdir -p /var/lib/o3k/images /var/lib/o3k/volumes && \
    chown -R o3k:o3k /var/lib/o3k

# Change ownership
RUN chown -R o3k:o3k /app

# Note: Running as root for network namespace operations in privileged mode
# The container must be run with privileged: true for real networking mode

# Expose ports
EXPOSE 5000 8774 9696 8776 9292 8775

# Health check (use internal port 35357)
HEALTHCHECK --interval=10s --timeout=5s --start-period=30s --retries=3 \
    CMD curl -f http://localhost:35357/v3 || exit 1

# Run migrations and start O3K
CMD ["/bin/sh", "-c", "/app/o3k-migrate up && /app/o3k --config /app/config/o3k.yaml"]
