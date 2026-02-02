.PHONY: build test lint clean install run release-snapshot help setup-hooks ci

# Binary name
BINARY_NAME := lazykamal
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

# Go parameters
GOCMD := go
GOBUILD := $(GOCMD) build
GOTEST := $(GOCMD) test
GOVET := $(GOCMD) vet
GOFMT := gofmt
GOMOD := $(GOCMD) mod

# Build flags
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

## help: Show this help message
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':' | sed 's/^/ /'

## build: Build the binary
build:
	$(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME) .

## test: Run tests with race detection
test:
	$(GOTEST) -v -race -coverprofile=coverage.out ./...

## test-short: Run tests without race detection (faster)
test-short:
	$(GOTEST) -v ./...

## lint: Run golangci-lint
lint:
	golangci-lint run

## lint-fix: Run golangci-lint with auto-fix
lint-fix:
	golangci-lint run --fix

## fmt: Format code
fmt:
	$(GOFMT) -s -w .

## vet: Run go vet
vet:
	$(GOVET) ./...

## clean: Remove build artifacts
clean:
	rm -f $(BINARY_NAME)
	rm -f coverage.out
	rm -rf dist/

## install: Install binary to GOPATH/bin
install:
	$(GOCMD) install $(LDFLAGS) .

## run: Build and run the application
run: build
	./$(BINARY_NAME)

## deps: Download dependencies
deps:
	$(GOMOD) download
	$(GOMOD) tidy

## release-snapshot: Create a snapshot release (no publish)
release-snapshot:
	goreleaser release --snapshot --clean

## check: Run all checks (fmt, vet, lint, test)
check: fmt vet lint test
	@echo "All checks passed!"

## coverage: Run tests and open coverage report
coverage: test
	$(GOCMD) tool cover -html=coverage.out

## setup-hooks: Install git hooks for local CI checks before push
setup-hooks:
	@echo "Setting up git hooks..."
	git config core.hooksPath .githooks
	@echo "✓ Git hooks installed! Pre-push checks will run automatically."
	@echo ""
	@echo "To disable: git config --unset core.hooksPath"

## ci: Run CI checks locally (same as GitHub Actions)
ci:
	@echo "Running CI checks locally..."
	@echo ""
	@echo "1/4 Checking formatting..."
	@test -z "$$(gofmt -l . | grep -v vendor)" || (echo "Files not formatted:" && gofmt -l . && exit 1)
	@echo "✓ Formatting OK"
	@echo ""
	@echo "2/4 Running go vet..."
	@$(GOVET) ./...
	@echo "✓ Vet OK"
	@echo ""
	@echo "3/4 Building..."
	@$(GOBUILD) -v ./...
	@echo "✓ Build OK"
	@echo ""
	@echo "4/4 Running tests..."
	@$(GOTEST) -race ./...
	@echo "✓ Tests OK"
	@echo ""
	@if command -v golangci-lint >/dev/null 2>&1; then \
		echo "5/5 Running golangci-lint..."; \
		golangci-lint run ./...; \
		echo "✓ Lint OK"; \
	else \
		echo "⚠ golangci-lint not installed, skipping"; \
		echo "  Install: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi
	@echo ""
	@echo "✓ All CI checks passed!"
