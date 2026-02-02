.PHONY: build test lint clean install run release-snapshot help

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
