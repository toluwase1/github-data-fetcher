# Makefile for github-data-fetcher

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOMOD=$(GOCMD) mod
BINARY_NAME=service

# Docker parameters
DOCKER_COMPOSE=docker-compose
DOCKER_BUILD=docker build

.PHONY: all build clean run test test-integration lint tidy docker-build up down logs help

all: build

# Build the application binary
build:
	@echo "Building binary..."
	$(GOBUILD) -o $(BINARY_NAME) ./cmd/service

# Clean the binary
clean:
	@echo "Cleaning up..."
	$(GOCLEAN)
	rm -f $(BINARY_NAME)

# Run the application directly
run: build
	@echo "Running application..."
	./$(BINARY_NAME)

# Run all unit tests (excluding integration tests)
test:
	@echo "Running unit tests..."
	$(GOTEST) -v -race ./...

# Run integration tests (requires Docker)
# The -tags=integration flag enables tests with the '//go:build integration' tag.
test-integration:
	@echo "Running integration tests..."
	$(GOTEST) -v -race -tags=integration ./...

# Tidy go.mod and go.sum files
tidy:
	@echo "Tidying go modules..."
	$(GOMOD) tidy

# Lint the codebase using golangci-lint
# You need to install it first: https://golangci-lint.run/usage/install/
lint:
	@echo "Linting code..."
	@command -v golangci-lint >/dev/null 2>&1 || { echo >&2 "golangci-lint is not installed. Please install it."; exit 1; }
	golangci-lint run

# Build the Docker image for the application
docker-build:
	@echo "Building Docker image..."
	$(DOCKER_BUILD) -t github-data-fetcher .

# Start all services with Docker Compose in detached mode
up:
	@echo "Starting services with docker-compose..."
	$(DOCKER_COMPOSE) up -d --build

# Stop and remove all services
down:
	@echo "Stopping services with docker-compose..."
	$(DOCKER_COMPOSE) down

# View logs for the app service
logs:
	@echo "Tailing application logs..."
	$(DOCKER_COMPOSE) logs -f app

# Display help
help:
	@echo ''
	@echo 'Usage:'
	@echo '  make [target]'
	@echo ''
	@echo 'Targets:'
	@echo '  build             Build the application binary'
	@echo '  run               Run the application directly (requires .env file)'
	@echo '  test              Run all unit tests'
	@echo '  test-integration  Run integration tests (requires Docker)'
	@echo '  lint              Lint the codebase'
	@echo '  tidy              Tidy go.mod and go.sum files'
	@echo '  docker-build      Build the Docker image'
	@echo '  up                Start all services with Docker Compose'
	@echo '  down              Stop and remove all services'
	@echo '  logs              View logs for the app service'
	@echo '  clean             Clean the binary'
	@echo ''