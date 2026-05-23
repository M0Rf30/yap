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
LDFLAGS=-ldflags="-s -w -X github.com/M0Rf30/yap/v2/pkg/buildinfo.Version=${VERSION} -X github.com/M0Rf30/yap/v2/pkg/buildinfo.BuildTime=${BUILD_TIME} -X github.com/M0Rf30/yap/v2/pkg/buildinfo.Commit=${COMMIT}"
BUILD_FLAGS=-trimpath $(LDFLAGS)

# Docker parameters
DOCKER_CMD=docker
DOCKER_REGISTRY ?= yap
DOCKER_TAG ?= latest
DOCKER_BUILD_FLAGS = --progress=plain --no-cache

# Available distributions (dynamically retrieved from build/deploy folder)
DISTROS = $(shell find build/deploy -mindepth 1 -maxdepth 1 -type d -exec basename {} \; | sort)

.PHONY: all build clean test test-coverage test-e2e-rpm bench deps fmt lint lint-md help run docker-build docker-build-all docker-list-distros doc doc-serve doc-package doc-deps doc-generate doc-serve-static i18n-tool i18n-check i18n-stats rpmdb-gen

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
	@$(GOTEST) -p 1 -v ./... || exit 1

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	$(GOTEST) -v -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Run E2E test for pure-Go RPM install on Rocky Linux 8
test-e2e-rpm:
	@echo "Running pure-Go RPM install E2E test on rockylinux:8..."
	@CONTAINER_RUNTIME=$$(command -v docker || command -v podman); \
	if [ -z "$$CONTAINER_RUNTIME" ]; then \
		echo "Error: docker or podman not found"; \
		echo "Please install Docker or Podman to run E2E tests"; \
		exit 1; \
	fi; \
	$$CONTAINER_RUNTIME run --rm \
	  -v $(PWD):/yap:Z \
	  -w /yap \
	  rockylinux:8 \
	  bash -c 'dnf -y install --setopt=install_weak_deps=False make sqlite curl tar gzip >/tmp/setup.log 2>&1 || (cat /tmp/setup.log; exit 1); GO_VERSION=1.26.0; if ! go version 2>/dev/null | grep -q "go$${GO_VERSION}"; then echo "==> Installing Go $${GO_VERSION}..."; curl -fsSL "https://go.dev/dl/go$${GO_VERSION}.linux-amd64.tar.gz" | tar -C /usr/local -xz; export PATH="/usr/local/go/bin:$$PATH"; fi; go version; make build && ./scripts/e2e-rpm.sh'

# Run benchmarks
bench:
	@echo "Running benchmarks..."
	$(GOTEST) -bench=. -benchmem -run=^$$ -benchtime=3s ./...

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
		markdownlint-cli2 --config .markdownlint.yml "**/*.md"; \
	elif command -v markdownlint > /dev/null; then \
		markdownlint --config .markdownlint.yml .; \
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

# Show localization statistics
i18n-stats:
	@echo "Showing localization statistics..."
	@$(GOCMD) run ./cmd/i18n-tool stats

# Check integrity of localization files
i18n-check:
	@echo "Checking localization file integrity..."
	@$(GOCMD) run ./cmd/i18n-tool check

# Build the i18n management tool
i18n-tool:
	@echo "Building i18n management tool..."
	@mkdir -p $(BUILD_DIR)
	@$(GOBUILD) -o $(BUILD_DIR)/i18n-tool ./cmd/i18n-tool
	@echo "i18n management tool built: $(BUILD_DIR)/i18n-tool"

# Regenerate the rpmdb sqlc bindings. Requires sqlc to be installed:
#   go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
rpmdb-gen:
	@echo "Regenerating rpmdb sqlc bindings..."
	@if command -v sqlc > /dev/null; then \
		sqlc generate; \
		echo "rpmdb bindings regenerated"; \
	elif command -v ~/go/bin/sqlc > /dev/null; then \
		~/go/bin/sqlc generate; \
		echo "rpmdb bindings regenerated"; \
	else \
		echo "sqlc not found. Install with: go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest"; \
		exit 1; \
	fi

# Help
help:
	@echo "Available targets:"
	@echo "  all              - Clean, deps, fmt, lint, test, doc, and build"
	@echo "  build            - Build the application"
	@echo "  clean            - Clean build artifacts"
	@echo "  test             - Run tests"
	@echo "  test-coverage    - Run tests with coverage report"
	@echo "  test-e2e-rpm     - Run E2E test for pure-Go RPM install on Rocky 8"
	@echo "  bench            - Run benchmarks"
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
	@echo "  i18n-tool        - Build the i18n management tool"
	@echo "  i18n-check       - Check integrity of localization files"
	@echo "  i18n-stats       - Show localization statistics"
	@echo "  rpmdb-gen        - Regenerate rpmdb sqlc bindings"
	@echo "  help             - Show this help"