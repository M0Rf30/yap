# Makefile for yap

# Variables
BINARY_NAME=yap
MAIN_PATH=./cmd/yap
BUILD_DIR=./bin
VERSION=$(shell git describe --tags --always --dirty)
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
COMMIT=$(shell git rev-parse HEAD)

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=gofmt
GOLINT=golangci-lint

# Build flags
LDFLAGS=-ldflags="-s -w -X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME} -X main.Commit=${COMMIT}"
BUILD_FLAGS=-trimpath $(LDFLAGS)

.PHONY: all build clean test deps fmt lint help install run dev doc doc-serve doc-package doc-deps doc-generate doc-serve-static

# Default target
all: clean deps fmt lint test build

# Build the application
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 $(GOBUILD) $(BUILD_FLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

# Clean build artifacts
clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	@rm -rf $(BUILD_DIR)
	@echo "Clean complete"

# Run tests
test:
	@echo "Running tests..."
	$(GOTEST) -v ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	$(GOTEST) -v -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Download dependencies
deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy

# Format code
fmt:
	@echo "Formatting code..."
	$(GOFMT) -s -w .

# Lint code
lint:
	@echo "Linting code..."
	@if command -v $(GOLINT) > /dev/null; then \
		$(GOLINT) run ./...; \
	else \
		echo "golangci-lint not installed, skipping lint"; \
	fi

# Run the application
run:
	@echo "Running $(BINARY_NAME)..."
	$(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	$(BUILD_DIR)/$(BINARY_NAME)

# Build for different architectures
build-linux-amd64:
	@echo "Building for Linux AMD64..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(BUILD_FLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(MAIN_PATH)

build-linux-arm64:
	@echo "Building for Linux ARM64..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=arm64 $(GOBUILD) $(BUILD_FLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 $(MAIN_PATH)

build-darwin-amd64:
	@echo "Building for macOS AMD64..."
	@mkdir -p $(BUILD_DIR)
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(BUILD_FLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 $(MAIN_PATH)

build-darwin-arm64:
	@echo "Building for macOS ARM64..."
	@mkdir -p $(BUILD_DIR)
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(BUILD_FLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 $(MAIN_PATH)

build-windows-amd64:
	@echo "Building for Windows AMD64..."
	@mkdir -p $(BUILD_DIR)
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(BUILD_FLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe $(MAIN_PATH)

# Build for all supported architectures
build-all: build-linux-amd64 build-linux-arm64 build-darwin-amd64 build-darwin-arm64 build-windows-amd64

# Security scan
security:
	@echo "Running security scan..."
	@if command -v $(GOLINT) > /dev/null; then \
		$(GOLINT) run --enable-only gosec ./...; \
	else \
		echo "golangci-lint not installed, skipping security scan"; \
	fi

# Benchmark tests
benchmark:
	@echo "Running benchmarks..."
	$(GOTEST) -bench=. -benchmem ./...

# Update dependencies
update-deps:
	@echo "Updating dependencies..."
	$(GOMOD) get -u ./...
	$(GOMOD) tidy

# Verify dependencies
verify:
	@echo "Verifying dependencies..."
	$(GOMOD) verify

# Check for outdated dependencies
outdated:
	@echo "Checking for outdated dependencies..."
	@$(GOCMD) list -u -m all

# Create a release
release: clean deps fmt lint test build-all
	@echo "Creating release..."
	@mkdir -p releases
	@tar -czf releases/$(BINARY_NAME)-linux-amd64.tar.gz -C $(BUILD_DIR) $(BINARY_NAME)-linux-amd64
	@tar -czf releases/$(BINARY_NAME)-linux-arm64.tar.gz -C $(BUILD_DIR) $(BINARY_NAME)-linux-arm64
	@tar -czf releases/$(BINARY_NAME)-darwin-amd64.tar.gz -C $(BUILD_DIR) $(BINARY_NAME)-darwin-amd64
	@tar -czf releases/$(BINARY_NAME)-darwin-arm64.tar.gz -C $(BUILD_DIR) $(BINARY_NAME)-darwin-arm64
	@zip -j releases/$(BINARY_NAME)-windows-amd64.zip $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe
	@echo "Release packages created in releases/"

# Development server with auto-reload (requires air)
dev:
	@echo "Starting development server..."
	@if command -v air > /dev/null; then \
		air; \
	else \
		echo "air not installed. Install with: go install github.com/cosmtrek/air@latest"; \
		echo "Falling back to regular run..."; \
		$(MAKE) run; \
	fi

# List supported distributions
list-distros: build
	@echo "Listing supported distributions..."
	$(BUILD_DIR)/$(BINARY_NAME) list-distros

# Run example build
example: build
	@echo "Running example build..."
	@if [ -f examples/yap/PKGBUILD ]; then \
		cd examples/yap && ../../$(BUILD_DIR)/$(BINARY_NAME) build; \
	else \
		echo "Example PKGBUILD not found"; \
	fi

# Documentation
doc:
	@echo "Viewing documentation for all packages..."
	@for pkg in $$(find ./pkg -name "*.go" -exec dirname {} \; | sort -u); do \
		echo "=== $$pkg ==="; \
		$(GOCMD) doc $$pkg || true; \
		echo; \
	done

doc-serve:
	@echo "Starting documentation server on http://localhost:8080..."
	@if command -v pkgsite > /dev/null; then \
		pkgsite -http=localhost:8080 .; \
	elif [ -f $(HOME)/go/bin/pkgsite ]; then \
		$(HOME)/go/bin/pkgsite -http=localhost:8080 .; \
	else \
		echo "pkgsite not found. Install with: go install golang.org/x/pkgsite/cmd/pkgsite@latest"; \
		echo "Falling back to go doc..."; \
		$(MAKE) doc; \
	fi

doc-package:
	@echo "Usage: make doc-package PKG=<package_path>"
	@if [ -z "$(PKG)" ]; then \
		echo "Example: make doc-package PKG=./pkg/source"; \
	else \
		$(GOCMD) doc -all $(PKG); \
	fi

# Install documentation tools
doc-deps:
	@echo "Installing documentation dependencies..."
	@$(GOCMD) install golang.org/x/pkgsite/cmd/pkgsite@latest
	@echo "Documentation tools installed"

# Generate static documentation files
doc-generate:
	@echo "Generating static documentation files..."
	@mkdir -p docs/api
	@for pkg in $$(find ./pkg -name "*.go" -exec dirname {} \; | sort -u); do \
		pkg_name=$$(basename $$pkg); \
		echo "Generating docs for $$pkg_name..."; \
		$(GOCMD) doc -all $$pkg > docs/api/$$pkg_name.txt 2>/dev/null || true; \
	done
	@echo "Documentation files generated in docs/api/"

# Serve static documentation files
doc-serve-static:
	@echo "Generating documentation files..."
	@$(MAKE) doc-generate
	@echo "Starting HTTP server for static docs on http://localhost:8081..."
	@echo "Navigate to http://localhost:8081/api/ to browse documentation files"
	@cd docs && python3 -m http.server 8081 2>/dev/null || python -m SimpleHTTPServer 8081

# Help
help:
	@echo "Available targets:"
	@echo "  all          - Clean, deps, fmt, lint, test, and build"
	@echo "  build        - Build the application"
	@echo "  clean        - Clean build artifacts"
	@echo "  test         - Run tests"
	@echo "  test-coverage- Run tests with coverage report"
	@echo "  deps         - Download dependencies"
	@echo "  fmt          - Format code"
	@echo "  lint         - Lint code"
	@echo "  run          - Build and run the application"
	@echo "  build-all    - Build for all supported architectures"
	@echo "  security     - Run security scan"
	@echo "  benchmark    - Run benchmark tests"
	@echo "  update-deps  - Update dependencies"
	@echo "  verify       - Verify dependencies"
	@echo "  outdated     - Check for outdated dependencies"
	@echo "  release      - Create release packages"
	@echo "  dev          - Start development server with auto-reload"
	@echo "  list-distros - List supported distributions"
	@echo "  example      - Run example build"
	@echo "  doc          - View all package documentation"
	@echo "  doc-serve    - Start documentation server on localhost:8080"
	@echo "  doc-package  - View specific package docs (use PKG=<path>)"
	@echo "  doc-deps     - Install documentation tools"
	@echo "  doc-generate - Generate static documentation files in docs/api/"
	@echo "  doc-serve-static - Serve static documentation files on localhost:8081"
	@echo "  help         - Show this help"