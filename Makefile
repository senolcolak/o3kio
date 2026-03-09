.PHONY: build run test clean install-deps migrate db-up db-down

# Build variables
BINARY_NAME=o3k
BUILD_DIR=bin
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-ldflags "-X main.Version=$(VERSION)"

# Database variables
DB_URL?=postgres://o3k:secret@localhost:5432/o3k?sslmode=disable

# Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/o3k

# Run the application
run: build
	@echo "Starting O3K..."
	./$(BUILD_DIR)/$(BINARY_NAME) --config config/o3k.yaml --migrations migrations

# Install dependencies
install-deps:
	@echo "Installing dependencies..."
	go mod download
	go mod tidy

# Run tests
test:
	@echo "Running tests..."
	go test -v ./...

# Run contract tests (requires O3K to be running)
test-contract:
	@echo "Running contract tests..."
	@echo "Checking O3K is running..."
	@curl -s http://localhost:35357/v3 > /dev/null || (echo "ERROR: O3K not running. Run 'docker compose up -d' or 'make run' first." && exit 1)
	@echo "Running gophercloud contract tests..."
	go test -v ./test/contract/... -timeout 5m

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -rf $(BUILD_DIR)

# Run database migrations manually
migrate:
	@echo "Running migrations..."
	migrate -path migrations -database "$(DB_URL)" up

# Start PostgreSQL in Docker (for development)
db-up:
	@echo "Starting PostgreSQL..."
	docker run -d --name o3k-postgres \
		-e POSTGRES_DB=o3k \
		-e POSTGRES_USER=o3k \
		-e POSTGRES_PASSWORD=secret \
		-p 5432:5432 \
		postgres:16
	@echo "Waiting for PostgreSQL to be ready..."
	@sleep 3

# Stop PostgreSQL
db-down:
	@echo "Stopping PostgreSQL..."
	docker stop o3k-postgres || true
	docker rm o3k-postgres || true

# Run with hot reload (requires air: go install github.com/cosmtrek/air@latest)
dev:
	@echo "Starting development mode with hot reload..."
	air

# Format code
fmt:
	@echo "Formatting code..."
	go fmt ./...

# Lint code
lint:
	@echo "Linting code..."
	golangci-lint run ./...

# Install development tools
install-tools:
	@echo "Installing development tools..."
	go install github.com/cosmtrek/air@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

# Show help
help:
	@echo "O3K Makefile targets:"
	@echo "  build          - Build the binary"
	@echo "  run            - Build and run the application"
	@echo "  test           - Run tests"
	@echo "  test-contract  - Run contract tests (requires O3K running)"
	@echo "  clean          - Remove build artifacts"
	@echo "  install-deps   - Install Go dependencies"
	@echo "  migrate        - Run database migrations"
	@echo "  db-up          - Start PostgreSQL in Docker"
	@echo "  db-down        - Stop PostgreSQL container"
	@echo "  dev            - Run with hot reload (requires air)"
	@echo "  fmt            - Format code"
	@echo "  lint           - Lint code"
	@echo "  install-tools  - Install development tools"
