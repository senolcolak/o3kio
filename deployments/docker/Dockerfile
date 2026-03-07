# Multi-stage build for O3K
FROM golang:1.26-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o o3k ./cmd/o3k

# Build the migration tool
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o o3k-migrate ./cmd/o3k-migrate

# Runtime stage
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache \
    ca-certificates \
    curl \
    tzdata

# Create non-root user
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

# Change ownership
RUN chown -R o3k:o3k /app

# Switch to non-root user
USER o3k

# Expose ports
EXPOSE 5000 8774 9696 8776 9292 8775

# Health check
HEALTHCHECK --interval=10s --timeout=5s --start-period=30s --retries=3 \
    CMD curl -f http://localhost:5000/v3 || exit 1

# Run migrations and start O3K
CMD ["/bin/sh", "-c", "/app/o3k-migrate up && /app/o3k --config /app/config/o3k.yaml"]
