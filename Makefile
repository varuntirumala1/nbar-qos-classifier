# NBAR QoS Classifier Makefile

# Build variables
BINARY_NAME=nbar-classifier
BINARY_PATH=./cmd/nbar-classifier
BUILD_DIR=./build
VERSION?=dev
GIT_COMMIT?=$(shell git rev-parse --short HEAD)
BUILD_TIME?=$(shell date -u '+%Y-%m-%d_%H:%M:%S')

# Go variables
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=$(GOCMD) fmt

# Build flags
LDFLAGS=-ldflags "-X main.Version=$(VERSION) -X main.GitCommit=$(GIT_COMMIT) -X main.BuildTime=$(BUILD_TIME)"

# Default target
.PHONY: all
all: clean deps fmt vet test build

# Install dependencies
.PHONY: deps
deps:
	$(GOMOD) download
	$(GOMOD) tidy

# Format code
.PHONY: fmt
fmt:
	$(GOFMT) ./...

# Vet code
.PHONY: vet
vet:
	$(GOCMD) vet ./...

# Run tests
.PHONY: test
test:
	$(GOTEST) -v ./test/unit/...

# Run tests with coverage
.PHONY: test-coverage
test-coverage:
	$(GOTEST) -v -coverprofile=coverage.out ./test/unit/...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

# Run integration tests
.PHONY: test-integration
test-integration:
	$(GOTEST) -v ./test/integration/...

# Run all tests
.PHONY: test-all
test-all: test test-integration

# Build the binary
.PHONY: build
build:
	mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(BINARY_PATH)

# Build for multiple platforms
.PHONY: build-all
build-all: build-linux build-darwin build-windows

.PHONY: build-linux
build-linux:
	mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(BINARY_PATH)

.PHONY: build-darwin
build-darwin:
	mkdir -p $(BUILD_DIR)
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 $(BINARY_PATH)
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 $(BINARY_PATH)

.PHONY: build-windows
build-windows:
	mkdir -p $(BUILD_DIR)
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe $(BINARY_PATH)

# Install the binary
.PHONY: install
install: build
	cp $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/

# Clean build artifacts
.PHONY: clean
clean:
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html

# Run the application
.PHONY: run
run: build
	$(BUILD_DIR)/$(BINARY_NAME) --config=./configs/config.yaml

# Run with development config
.PHONY: run-dev
run-dev: build
	$(BUILD_DIR)/$(BINARY_NAME) --config=./configs/config.yaml --log-level=debug --enable-metrics --enable-web

# Docker targets
.PHONY: docker-build
docker-build:
	docker build -t nbar-classifier:$(VERSION) -f deployments/docker/Dockerfile .

.PHONY: docker-run
docker-run:
	docker run --rm -it -p 8080:8080 -p 9090:9090 nbar-classifier:$(VERSION)

# Kubernetes targets
.PHONY: k8s-deploy
k8s-deploy:
	kubectl apply -f deployments/k8s/

.PHONY: k8s-delete
k8s-delete:
	kubectl delete -f deployments/k8s/

# Development helpers
.PHONY: dev-setup
dev-setup:
	$(GOGET) github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	$(GOGET) github.com/air-verse/air@latest

# Lint code
.PHONY: lint
lint:
	golangci-lint run

# Watch and rebuild on changes
.PHONY: watch
watch:
	air -c .air.toml

# Generate documentation
.PHONY: docs
docs:
	$(GOCMD) doc -all ./pkg/... > docs/api.md

# Security scan
.PHONY: security
security:
	$(GOCMD) list -json -m all | nancy sleuth

# Benchmark tests
.PHONY: bench
bench:
	$(GOTEST) -bench=. -benchmem ./test/unit/...

# Profile the application
.PHONY: profile
profile: build
	$(BUILD_DIR)/$(BINARY_NAME) --config=./configs/config.yaml --cpuprofile=cpu.prof --memprofile=mem.prof

# Update dependencies
.PHONY: update-deps
update-deps:
	$(GOGET) -u ./...
	$(GOMOD) tidy

# Check for vulnerabilities
.PHONY: vuln-check
vuln-check:
	$(GOCMD) list -json -m all | nancy sleuth

# Release preparation
.PHONY: release-prep
release-prep: clean deps fmt vet lint test-all build-all

# Show help
.PHONY: help
help:
	@echo "Available targets:"
	@echo "  all           - Run clean, deps, fmt, vet, test, and build"
	@echo "  deps          - Install dependencies"
	@echo "  fmt           - Format code"
	@echo "  vet           - Vet code"
	@echo "  lint          - Lint code with golangci-lint"
	@echo "  test          - Run unit tests"
	@echo "  test-coverage - Run tests with coverage report"
	@echo "  test-integration - Run integration tests"
	@echo "  test-all      - Run all tests"
	@echo "  build         - Build the binary"
	@echo "  build-all     - Build for all platforms"
	@echo "  install       - Install the binary to /usr/local/bin"
	@echo "  run           - Build and run the application"
	@echo "  run-dev       - Run with development settings"
	@echo "  docker-build  - Build Docker image"
	@echo "  docker-run    - Run Docker container"
	@echo "  k8s-deploy    - Deploy to Kubernetes"
	@echo "  k8s-delete    - Delete from Kubernetes"
	@echo "  clean         - Clean build artifacts"
	@echo "  docs          - Generate documentation"
	@echo "  security      - Run security scan"
	@echo "  bench         - Run benchmark tests"
	@echo "  profile       - Profile the application"
	@echo "  update-deps   - Update dependencies"
	@echo "  vuln-check    - Check for vulnerabilities"
	@echo "  release-prep  - Prepare for release"
	@echo "  help          - Show this help message"

# Default target when no target is specified
.DEFAULT_GOAL := help
