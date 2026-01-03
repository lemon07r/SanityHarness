# =============================================================================
# SanityHarness - Production Makefile
# =============================================================================
# Modern Go project Makefile with self-documenting help, cross-compilation,
# and comprehensive tooling for development, testing, and CI/CD.
#
# Usage: make <target>
# Run 'make' or 'make help' to see all available targets.
# =============================================================================

# -----------------------------------------------------------------------------
# Configuration
# -----------------------------------------------------------------------------

# Binary name and paths
BINARY_NAME   := sanity
CMD_PATH      := ./cmd/sanity
BIN_DIR       := ./bin
COVERAGE_DIR  := ./coverage

# Go module path (for ldflags injection)
MODULE_PATH   := github.com/lemon07r/sanityharness
CLI_PKG       := $(MODULE_PATH)/internal/cli

# Build info (injected at compile time)
VERSION       ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT_HASH   := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME    := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')

# Go build flags
LDFLAGS := -s -w \
	-X '$(CLI_PKG).Version=$(VERSION)' \
	-X '$(CLI_PKG).Commit=$(COMMIT_HASH)' \
	-X '$(CLI_PKG).BuildDate=$(BUILD_TIME)'

# Platforms for cross-compilation
PLATFORMS := linux/amd64 darwin/arm64 windows/amd64

# Tool versions (for CI reproducibility)
GOLANGCI_LINT_VERSION := v2.1.6

# -----------------------------------------------------------------------------
# ANSI Color Codes
# -----------------------------------------------------------------------------

NO_COLOR    := \033[0m
BOLD        := \033[1m
GREEN       := \033[32m
YELLOW      := \033[33m
BLUE        := \033[34m
MAGENTA     := \033[35m
CYAN        := \033[36m
WHITE       := \033[37m

# Styled prefixes
INFO        := $(BLUE)[INFO]$(NO_COLOR)
OK          := $(GREEN)[OK]$(NO_COLOR)
WARN        := $(YELLOW)[WARN]$(NO_COLOR)
BUILD       := $(MAGENTA)[BUILD]$(NO_COLOR)
TEST        := $(CYAN)[TEST]$(NO_COLOR)

# -----------------------------------------------------------------------------
# Default Target
# -----------------------------------------------------------------------------

.DEFAULT_GOAL := help

# -----------------------------------------------------------------------------
# Help (Self-Documenting)
# -----------------------------------------------------------------------------

.PHONY: help
help: ## Display this help message
	@echo ''
	@echo '$(BOLD)$(CYAN)SanityHarness$(NO_COLOR) - Lightweight evaluation harness for coding agents'
	@echo ''
	@echo '$(BOLD)Usage:$(NO_COLOR)'
	@echo '  make $(YELLOW)<target>$(NO_COLOR)'
	@echo ''
	@echo '$(BOLD)Targets:$(NO_COLOR)'
	@awk 'BEGIN {FS = ":.*##"; printf ""} /^[a-zA-Z_-]+:.*?##/ { printf "  $(YELLOW)%-18s$(NO_COLOR) %s\n", $$1, $$2 } /^##@/ { printf "\n$(BOLD)%s$(NO_COLOR)\n", substr($$0, 5) }' $(MAKEFILE_LIST)
	@echo ''
	@echo '$(BOLD)Build Info:$(NO_COLOR)'
	@echo '  Version:    $(VERSION)'
	@echo '  Commit:     $(COMMIT_HASH)'
	@echo '  Build Time: $(BUILD_TIME)'
	@echo ''

##@ Development

.PHONY: build
build: ## Build the binary for current platform
	@printf '$(BUILD) Building $(BINARY_NAME)...\n'
	@go build -ldflags "$(LDFLAGS)" -o $(BINARY_NAME) $(CMD_PATH)
	@printf '$(OK) Built: ./$(BINARY_NAME)\n'

.PHONY: build-debug
build-debug: ## Build with debug symbols (no stripping)
	@printf '$(BUILD) Building $(BINARY_NAME) (debug)...\n'
	@go build -ldflags "-X '$(CLI_PKG).Version=$(VERSION)' -X '$(CLI_PKG).Commit=$(COMMIT_HASH)' -X '$(CLI_PKG).BuildDate=$(BUILD_TIME)'" -o $(BINARY_NAME) $(CMD_PATH)
	@printf '$(OK) Built: ./$(BINARY_NAME) (with debug symbols)\n'

.PHONY: run
run: build ## Build and run the binary
	@printf '$(INFO) Running $(BINARY_NAME)...\n'
	@./$(BINARY_NAME)

.PHONY: clean
clean: ## Remove build artifacts and generated files
	@printf '$(INFO) Cleaning build artifacts...\n'
	@rm -f $(BINARY_NAME)
	@rm -rf $(BIN_DIR)
	@rm -rf $(COVERAGE_DIR)
	@rm -f coverage.out coverage.html
	@go clean -cache -testcache
	@printf '$(OK) Clean complete\n'

##@ Dependencies

.PHONY: deps
deps: ## Download module dependencies
	@printf '$(INFO) Downloading dependencies...\n'
	@go mod download
	@printf '$(OK) Dependencies downloaded\n'

.PHONY: tidy
tidy: ## Tidy go.mod and go.sum
	@printf '$(INFO) Tidying modules...\n'
	@go mod tidy
	@printf '$(OK) Modules tidied\n'

.PHONY: verify
verify: ## Verify dependencies
	@printf '$(INFO) Verifying dependencies...\n'
	@go mod verify
	@printf '$(OK) Dependencies verified\n'

##@ Code Quality

.PHONY: fmt
fmt: ## Format code with goimports (handles import sorting)
	@printf '$(INFO) Formatting code with goimports...\n'
	@if command -v goimports >/dev/null 2>&1; then \
		goimports -w -local $(MODULE_PATH) .; \
	else \
		printf '$(WARN) goimports not found, falling back to gofmt...\n'; \
		go fmt ./...; \
	fi
	@printf '$(OK) Code formatted\n'

.PHONY: lint
lint: ## Run golangci-lint
	@printf '$(INFO) Running golangci-lint...\n'
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		printf '$(WARN) golangci-lint not found. Install: go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)\n'; \
		exit 1; \
	fi
	@printf '$(OK) Linting passed\n'

.PHONY: vet
vet: ## Run go vet
	@printf '$(INFO) Running go vet...\n'
	@go vet ./...
	@printf '$(OK) Vet passed\n'

.PHONY: vuln-check
vuln-check: ## Run govulncheck for security vulnerabilities
	@printf '$(INFO) Running govulncheck...\n'
	@if command -v govulncheck >/dev/null 2>&1; then \
		govulncheck ./...; \
	else \
		printf '$(WARN) govulncheck not found. Install: go install golang.org/x/vuln/cmd/govulncheck@latest\n'; \
		exit 1; \
	fi
	@printf '$(OK) Vulnerability check complete\n'

.PHONY: check
check: fmt vet lint ## Run all code quality checks (fmt, vet, lint)
	@printf '$(OK) All checks passed\n'

##@ Testing

.PHONY: test
test: ## Run tests with race detection
	@printf '$(TEST) Running tests with race detection...\n'
	@go test -race -v ./...
	@printf '$(OK) Tests passed\n'

.PHONY: test-short
test-short: ## Run tests in short mode (skip long-running tests)
	@printf '$(TEST) Running short tests...\n'
	@go test -race -short -v ./...
	@printf '$(OK) Short tests passed\n'

.PHONY: coverage
coverage: ## Run tests with coverage and generate HTML report
	@printf '$(TEST) Running tests with coverage...\n'
	@mkdir -p $(COVERAGE_DIR)
	@go test -race -coverprofile=$(COVERAGE_DIR)/coverage.out -covermode=atomic ./...
	@go tool cover -html=$(COVERAGE_DIR)/coverage.out -o $(COVERAGE_DIR)/coverage.html
	@go tool cover -func=$(COVERAGE_DIR)/coverage.out | tail -1
	@printf '$(OK) Coverage report: $(COVERAGE_DIR)/coverage.html\n'

.PHONY: coverage-open
coverage-open: coverage ## Generate and open coverage report in browser
	@printf '$(INFO) Opening coverage report...\n'
	@if command -v xdg-open >/dev/null 2>&1; then \
		xdg-open $(COVERAGE_DIR)/coverage.html; \
	elif command -v open >/dev/null 2>&1; then \
		open $(COVERAGE_DIR)/coverage.html; \
	else \
		printf '$(WARN) Cannot open browser. Report at: $(COVERAGE_DIR)/coverage.html\n'; \
	fi

.PHONY: bench
bench: ## Run benchmarks
	@printf '$(TEST) Running benchmarks...\n'
	@go test -bench=. -benchmem ./...
	@printf '$(OK) Benchmarks complete\n'

##@ Build (Cross-Compilation)

.PHONY: build-all
build-all: ## Build for all platforms (Linux/amd64, Darwin/arm64, Windows/amd64)
	@printf '$(BUILD) Building for all platforms...\n'
	@mkdir -p $(BIN_DIR)
	@for platform in $(PLATFORMS); do \
		GOOS=$$(echo $$platform | cut -d'/' -f1); \
		GOARCH=$$(echo $$platform | cut -d'/' -f2); \
		output=$(BIN_DIR)/$(BINARY_NAME)-$$GOOS-$$GOARCH; \
		if [ "$$GOOS" = "windows" ]; then output=$$output.exe; fi; \
		printf '\033[35m[BUILD]\033[0m Building %s/%s -> %s\n' "$$GOOS" "$$GOARCH" "$$output"; \
		GOOS=$$GOOS GOARCH=$$GOARCH go build -ldflags "$(LDFLAGS)" -o $$output $(CMD_PATH); \
	done
	@printf '$(OK) Cross-compilation complete. Binaries in $(BIN_DIR)/\n'
	@ls -la $(BIN_DIR)/

.PHONY: build-linux
build-linux: ## Build for Linux/amd64
	@printf '$(BUILD) Building for Linux/amd64...\n'
	@mkdir -p $(BIN_DIR)
	@GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY_NAME)-linux-amd64 $(CMD_PATH)
	@printf '$(OK) Built: $(BIN_DIR)/$(BINARY_NAME)-linux-amd64\n'

.PHONY: build-darwin
build-darwin: ## Build for Darwin/arm64 (Apple Silicon)
	@printf '$(BUILD) Building for Darwin/arm64...\n'
	@mkdir -p $(BIN_DIR)
	@GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY_NAME)-darwin-arm64 $(CMD_PATH)
	@printf '$(OK) Built: $(BIN_DIR)/$(BINARY_NAME)-darwin-arm64\n'

.PHONY: build-windows
build-windows: ## Build for Windows/amd64
	@printf '$(BUILD) Building for Windows/amd64...\n'
	@mkdir -p $(BIN_DIR)
	@GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY_NAME)-windows-amd64.exe $(CMD_PATH)
	@printf '$(OK) Built: $(BIN_DIR)/$(BINARY_NAME)-windows-amd64.exe\n'

##@ Tools

.PHONY: tools
tools: ## Install development tools (goimports, golangci-lint, govulncheck)
	@printf '$(INFO) Installing development tools...\n'
	@printf '$(INFO) Installing goimports...\n'
	@go install golang.org/x/tools/cmd/goimports@latest
	@printf '$(INFO) Installing golangci-lint $(GOLANGCI_LINT_VERSION)...\n'
	@go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	@printf '$(INFO) Installing govulncheck...\n'
	@go install golang.org/x/vuln/cmd/govulncheck@latest
	@printf '$(OK) All tools installed\n'

.PHONY: tools-check
tools-check: ## Check if required tools are installed
	@printf '$(INFO) Checking required tools...\n'
	@printf '  goimports:     '; command -v goimports >/dev/null 2>&1 && printf '$(GREEN)installed$(NO_COLOR)\n' || printf '$(YELLOW)missing$(NO_COLOR)\n'
	@printf '  golangci-lint: '; command -v golangci-lint >/dev/null 2>&1 && printf '$(GREEN)installed$(NO_COLOR)\n' || printf '$(YELLOW)missing$(NO_COLOR)\n'
	@printf '  govulncheck:   '; command -v govulncheck >/dev/null 2>&1 && printf '$(GREEN)installed$(NO_COLOR)\n' || printf '$(YELLOW)missing$(NO_COLOR)\n'

##@ Docker

.PHONY: docker-build
docker-build: ## Build all Docker images for task execution
	@printf '$(INFO) Building Docker images...\n'
	@docker build -f containers/Dockerfile-go -t ghcr.io/lemon07r/sanity-go:latest .
	@docker build -f containers/Dockerfile-rust -t ghcr.io/lemon07r/sanity-rust:latest .
	@docker build -f containers/Dockerfile-ts -t ghcr.io/lemon07r/sanity-ts:latest .
	@printf '$(OK) Docker images built\n'

.PHONY: docker-push
docker-push: ## Push Docker images to GHCR
	@printf '$(INFO) Pushing Docker images to GHCR...\n'
	@docker push ghcr.io/lemon07r/sanity-go:latest
	@docker push ghcr.io/lemon07r/sanity-rust:latest
	@docker push ghcr.io/lemon07r/sanity-ts:latest
	@printf '$(OK) Docker images pushed\n'

##@ CI/CD

.PHONY: ci
ci: deps check test build ## Run full CI pipeline (deps, check, test, build)
	@printf '$(OK) CI pipeline complete\n'

.PHONY: ci-full
ci-full: deps check test vuln-check coverage build-all ## Run extended CI with security and coverage
	@printf '$(OK) Full CI pipeline complete\n'

.PHONY: pre-commit
pre-commit: fmt vet lint test-short ## Run pre-commit checks (fast)
	@printf '$(OK) Pre-commit checks passed\n'

##@ Release

.PHONY: release-dry
release-dry: clean build-all ## Dry run for release (clean + build all)
	@printf '$(INFO) Release dry run complete\n'
	@printf '$(INFO) Version: $(VERSION)\n'
	@printf '$(INFO) Commit:  $(COMMIT_HASH)\n'
	@printf '$(INFO) Artifacts:\n'
	@ls -la $(BIN_DIR)/

.PHONY: version
version: ## Print version information
	@printf 'Version:    $(VERSION)\n'
	@printf 'Commit:     $(COMMIT_HASH)\n'
	@printf 'Build Time: $(BUILD_TIME)\n'
	@printf 'Go Version: '
	@go version | awk '{print $$3}'
