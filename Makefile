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
GOMOD=$(GOCMD) mod
GOFMT=gofmt
GOLINT=golangci-lint

# Build flags
LDFLAGS=-ldflags="-s -w -X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME} -X main.Commit=${COMMIT}"
BUILD_FLAGS=-trimpath $(LDFLAGS)

# Docker parameters
DOCKER_CMD=docker
DOCKER_REGISTRY ?= yap
DOCKER_TAG ?= latest
DOCKER_BUILD_FLAGS = --progress=plain --no-cache

# Available distributions (dynamically retrieved from build/deploy folder)
DISTROS = $(shell find build/deploy -mindepth 1 -maxdepth 1 -type d -exec basename {} \; | sort)

.PHONY: all build clean test deps fmt lint lint-md help run docker-build docker-build-all docker-list-distros doc doc-serve doc-package doc-deps doc-generate doc-serve-static

# Default target
all: clean deps fmt lint lint-md test doc build

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

# Run tests
test:
	@echo "Running tests..."
	$(GOTEST) -v ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	$(GOTEST) -v -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

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

# Lint markdown files
lint-md:
	@echo "Linting markdown files..."
	@if command -v markdownlint-cli2 > /dev/null; then \
		markdownlint-cli2 "**/*.md"; \
	elif command -v markdownlint > /dev/null; then \
		markdownlint .; \
	else \
		echo "markdownlint not installed, skipping markdown lint"; \
		echo "Install with: npm install -g markdownlint-cli2"; \
	fi

# Generate documentation
doc:
	@echo "Viewing all package documentation..."
	@$(GOCMD) doc -all .

# Start documentation server
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

# View specific package documentation
doc-package:
	@echo "Usage: make doc-package PKG=<package_path>"
	@if [ -z "$(PKG)" ]; then \
		echo "Example: make doc-package PKG=./pkg/source"; \
	else \
		$(GOCMD) doc -all $(PKG); \
	fi

# Install documentation dependencies
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

# Run the application
run:
	@echo "Running $(BINARY_NAME)..."
	CGO_ENABLED=0 $(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	$(BUILD_DIR)/$(BINARY_NAME)

# Build for all architectures
build-all:
	@echo "Building for all architectures..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) $(BUILD_FLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(MAIN_PATH)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GOBUILD) $(BUILD_FLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 $(MAIN_PATH)
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GOBUILD) $(BUILD_FLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 $(MAIN_PATH)
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GOBUILD) $(BUILD_FLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 $(MAIN_PATH)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GOBUILD) $(BUILD_FLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe $(MAIN_PATH)

# Create release packages
release: clean deps fmt lint lint-md test doc build-all
	@echo "Creating release packages..."
	@mkdir -p releases
	@tar -czf releases/$(BINARY_NAME)-linux-amd64.tar.gz -C $(BUILD_DIR) $(BINARY_NAME)-linux-amd64
	@tar -czf releases/$(BINARY_NAME)-linux-arm64.tar.gz -C $(BUILD_DIR) $(BINARY_NAME)-linux-arm64
	@tar -czf releases/$(BINARY_NAME)-darwin-amd64.tar.gz -C $(BUILD_DIR) $(BINARY_NAME)-darwin-amd64
	@tar -czf releases/$(BINARY_NAME)-darwin-arm64.tar.gz -C $(BUILD_DIR) $(BINARY_NAME)-darwin-arm64
	@zip -j releases/$(BINARY_NAME)-windows-amd64.zip $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe
	@echo "Release packages created in releases/"

# Build Docker image for specific distribution
# Usage: make docker-build DISTRO=alpine
docker-build:
	@if [ -z "$(DISTRO)" ]; then \
		echo "Error: DISTRO parameter required"; \
		echo "Usage: make docker-build DISTRO=<distro>"; \
		echo "Available: $(DISTROS)"; \
		exit 1; \
	fi
	@if [ ! -d "build/deploy/$(DISTRO)" ]; then \
		echo "Error: Distribution '$(DISTRO)' not found"; \
		echo "Available: $(DISTROS)"; \
		exit 1; \
	fi
	@echo "Building Docker image for $(DISTRO)..."
	$(DOCKER_CMD) build $(DOCKER_BUILD_FLAGS) \
		-t $(DOCKER_REGISTRY):$(DISTRO)-$(DOCKER_TAG) \
		-f build/deploy/$(DISTRO)/Dockerfile .

# Build Docker images for all distributions
docker-build-all:
	@echo "Building Docker images for all distributions..."
	@for distro in $(DISTROS); do \
		echo "Building $$distro..."; \
		$(MAKE) docker-build DISTRO=$$distro || exit 1; \
	done

# List available Docker distributions
docker-list-distros:
	@echo "Available distributions:"
	@for distro in $(DISTROS); do echo "  $$distro"; done
	@echo "\nUsage:"
	@echo "  make docker-build DISTRO=<distro>"
	@echo "  make docker-build-all"

# Help
help:
	@echo "Available targets:"
	@echo "  all              - Clean, deps, fmt, lint, test, doc, and build"
	@echo "  build            - Build the application"
	@echo "  clean            - Clean build artifacts"
	@echo "  test             - Run tests"
	@echo "  test-coverage    - Run tests with coverage report"
	@echo "  deps             - Download dependencies"
	@echo "  fmt              - Format code"
	@echo "  lint             - Lint code"
	@echo "  lint-md          - Lint markdown files"
	@echo "  doc              - View all package documentation"
	@echo "  doc-serve        - Start documentation server on localhost:8080"
	@echo "  doc-package      - View specific package docs (use PKG=<path>)"
	@echo "  doc-deps         - Install documentation tools"
	@echo "  doc-generate     - Generate static documentation files in docs/api/"
	@echo "  doc-serve-static - Serve static documentation files on localhost:8081"
	@echo "  run              - Build and run the application"
	@echo "  build-all        - Build for all architectures"
	@echo "  release          - Create release packages"
	@echo "  docker-build DISTRO=<name> - Build Docker image for distribution"
	@echo "  docker-build-all - Build Docker images for all distributions"
	@echo "  docker-list-distros - List available distributions"
	@echo "  help             - Show this help"