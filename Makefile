.PHONY: build run test clean install-deps migrate db-up db-down build-ebpf bench bench-quick

# Build variables
BINARY_NAME=o3k
BUILD_DIR=bin
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-ldflags "-X main.Version=$(VERSION)"

# Database variables
POSTGRES_VERSION?=18
# Development-only defaults. Do NOT use in production.
# Production should set DB_URL via environment variable with sslmode=require.
DB_URL?=postgres://o3k:secret@localhost:5432/o3k?sslmode=disable

# Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/o3k

# Build eBPF programs
build-ebpf:
	@echo "Building eBPF programs..."
	@which clang > /dev/null || (echo "ERROR: clang not found. Install with: apt-get install clang llvm libbpf-dev" && exit 1)
	clang -O2 -target bpf -c pkg/networking/ebpf/secgroup.c -o pkg/networking/ebpf/secgroup.o
	@echo "eBPF program compiled: pkg/networking/ebpf/secgroup.o"

# Build with eBPF support
build-with-ebpf: build-ebpf build
	@echo "Built O3K with eBPF support"

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
	@curl -s http://localhost:35357/v3 > /dev/null || (echo "ERROR: O3K not running. Run 'docker compose -f deployments/docker-compose.yml up -d' or 'make run' first." && exit 1)
	@echo "Running gophercloud contract tests..."
	go test -v ./test/contract/... -timeout 5m

# Run integration tests (requires O3K to be running)
test-integration:
	@echo "Running integration tests..."
	@echo "Checking O3K is running..."
	@curl -s http://localhost:35357/v3 > /dev/null || (echo "ERROR: O3K not running. Run 'docker compose -f deployments/docker-compose.yml up -d' first." && exit 1)
	@echo "Running integration test suite..."
	@bash test/integration_test.sh

# Run multi-tenancy tests
test-multi-tenant:
	@echo "Running multi-tenancy tests..."
	@echo "Checking O3K is running..."
	@curl -s http://localhost:35357/v3 > /dev/null || (echo "ERROR: O3K not running." && exit 1)
	@bash test/multi_tenant_test.sh

# Run performance tests
test-performance:
	@echo "Running performance tests..."
	@echo "Checking O3K is running..."
	@curl -s http://localhost:35357/v3 > /dev/null || (echo "ERROR: O3K not running." && exit 1)
	@bash test/performance_test.sh

# Run error handling tests
test-errors:
	@echo "Running error handling tests..."
	@echo "Checking O3K is running..."
	@curl -s http://localhost:35357/v3 > /dev/null || (echo "ERROR: O3K not running." && exit 1)
	@bash test/error_handling_test.sh

# Run all tests (unit + contract + integration + performance)
test-all: test test-contract test-integration test-multi-tenant test-errors test-performance
	@echo ""
	@echo "All tests completed!"

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -rf $(BUILD_DIR)

# Run comprehensive performance benchmarks (Sprint 69)
bench:
	@echo "Running comprehensive performance benchmarks..."
	@echo "Checking O3K is running..."
	@curl -s http://localhost:35357/v3 > /dev/null || (echo "ERROR: O3K not running. Run 'docker compose -f deployments/docker-compose.yml up -d' first." && exit 1)
	@bash test/benchmark/run_benchmarks.sh

# Run quick benchmarks (Go only, no k6)
bench-quick:
	@echo "Running quick Go benchmarks..."
	@echo "Cache benchmarks:"
	@go test -bench=BenchmarkCache -benchmem -benchtime=5s ./test/benchmark/ || echo "Skipped (Redis not available)"
	@echo ""
	@echo "Database benchmarks:"
	@go test -bench=BenchmarkDatabase -benchmem -benchtime=5s ./test/benchmark/ || echo "Skipped (PostgreSQL not available)"

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
		postgres:$(POSTGRES_VERSION)
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

# Install eBPF development tools (Linux only)
install-ebpf-tools:
	@echo "Installing eBPF development tools..."
	@echo "NOTE: This requires Linux. On macOS/Windows, O3K runs in stub mode."
	@if [ "$$(uname)" = "Linux" ]; then \
		echo "Installing clang, llvm, libbpf-dev..."; \
		sudo apt-get update && sudo apt-get install -y clang llvm libbpf-dev linux-headers-$$(uname -r); \
		go get github.com/cilium/ebpf@latest; \
		echo "eBPF tools installed successfully"; \
	else \
		echo "Not on Linux - eBPF tools not needed for stub mode"; \
	fi

# Show help
help:
	@echo "O3K Makefile targets:"
	@echo "  build            - Build the binary"
	@echo "  build-ebpf       - Compile eBPF programs (Linux only)"
	@echo "  build-with-ebpf  - Build binary with eBPF support"
	@echo "  run              - Build and run the application"
	@echo "  test             - Run unit tests"
	@echo "  test-contract    - Run contract tests (requires O3K running)"
	@echo "  test-integration - Run integration tests"
	@echo "  test-multi-tenant - Run multi-tenancy isolation tests"
	@echo "  test-performance - Run performance and load tests"
	@echo "  test-errors      - Run error handling tests"
	@echo "  test-all         - Run all test suites"
	@echo "  clean            - Remove build artifacts"
	@echo "  install-deps     - Install Go dependencies"
	@echo "  migrate        - Run database migrations"
	@echo "  db-up          - Start PostgreSQL in Docker"
	@echo "  db-down        - Stop PostgreSQL container"
	@echo "  dev            - Run with hot reload (requires air)"
	@echo "  fmt            - Format code"
	@echo "  lint           - Lint code"
	@echo "  install-tools  - Install development tools"
	@echo "  install-ebpf-tools - Install eBPF development tools (Linux only)"
