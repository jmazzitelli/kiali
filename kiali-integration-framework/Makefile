# Makefile for Kiali Integration Test Framework

.PHONY: build clean test lint fmt vet mod-tidy help

# Build variables
BINARY_NAME=kiali-test
MAIN_FILE=cmd/main.go
BUILD_DIR=./build
DIST_DIR=./dist

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=$(GOCMD) fmt
GOVET=$(GOCMD) vet

# Build the binary
build:
	$(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_FILE)

# Build for multiple platforms
build-all: clean
	mkdir -p $(DIST_DIR)
	GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(DIST_DIR)/$(BINARY_NAME)-linux-amd64 $(MAIN_FILE)
	GOOS=darwin GOARCH=amd64 $(GOBUILD) -o $(DIST_DIR)/$(BINARY_NAME)-darwin-amd64 $(MAIN_FILE)
	GOOS=darwin GOARCH=arm64 $(GOBUILD) -o $(DIST_DIR)/$(BINARY_NAME)-darwin-arm64 $(MAIN_FILE)
	GOOS=windows GOARCH=amd64 $(GOBUILD) -o $(DIST_DIR)/$(BINARY_NAME)-windows-amd64.exe $(MAIN_FILE)

# Clean build artifacts
clean:
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)
	rm -rf $(DIST_DIR)

# Run tests
test:
	$(GOTEST) -v ./...

# Run tests with coverage
test-coverage:
	$(GOTEST) -v -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

# Format code
fmt:
	$(GOFMT) ./...

# Vet code
vet:
	$(GOVET) ./...

# Tidy dependencies
mod-tidy:
	$(GOMOD) tidy

# Install dependencies
deps:
	$(GOGET) -u ./...

# Lint code (requires golangci-lint)
lint:
	golangci-lint run

# Development setup
dev-setup: mod-tidy fmt vet
	@echo "Development environment setup complete"

# CI build
ci: mod-tidy fmt vet test build

# Install the binary to GOPATH/bin
install:
	$(GOBUILD) -o $(shell go env GOPATH)/bin/$(BINARY_NAME) $(MAIN_FILE)

# Run the application
run:
	$(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_FILE)
	./$(BUILD_DIR)/$(BINARY_NAME)

# Docker build
docker-build:
	docker build -t kiali-integration-framework .

# Docker run
docker-run:
	docker run --rm kiali-integration-framework

# Show help
help:
	@echo "Available targets:"
	@echo "  build        - Build the binary"
	@echo "  build-all    - Build for multiple platforms"
	@echo "  clean        - Clean build artifacts"
	@echo "  test         - Run tests"
	@echo "  test-coverage- Run tests with coverage report"
	@echo "  fmt          - Format code"
	@echo "  vet          - Vet code"
	@echo "  mod-tidy     - Tidy dependencies"
	@echo "  deps         - Install/update dependencies"
	@echo "  lint         - Lint code (requires golangci-lint)"
	@echo "  dev-setup    - Setup development environment"
	@echo "  ci           - CI build (format, vet, test, build)"
	@echo "  install      - Install binary to GOPATH/bin"
	@echo "  run          - Build and run the application"
	@echo "  docker-build - Build Docker image"
	@echo "  docker-run   - Run Docker container"
	@echo "  help         - Show this help"

.DEFAULT_GOAL := build
