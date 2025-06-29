# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
BINARY_NAME=upd-loader-bot
BINARY_UNIX=$(BINARY_NAME)_unix

# Docker parameters
DOCKER_IMAGE=upd-loader-bot
DOCKER_TAG=latest

.PHONY: all build clean test coverage deps run docker-build docker-run docker-stop help

all: test build

## Build the application
build:
	$(GOBUILD) -o $(BINARY_NAME) -v cmd/main.go

## Build for Linux
build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(BINARY_UNIX) -v cmd/main.go

## Clean build artifacts
clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_UNIX)

## Run tests
test:
	$(GOTEST) -v ./...

## Run tests with coverage
coverage:
	$(GOTEST) -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

## Download dependencies
deps:
	$(GOMOD) download
	$(GOMOD) tidy

## Initialize Go module (creates go.sum)
init:
	$(GOMOD) download
	$(GOMOD) tidy
	@echo "Go module initialized. You can now run Docker commands."

## Run the application locally
run:
	$(GOCMD) run cmd/main.go

## Run with live reload (requires air)
dev:
	@which air > /dev/null || (echo "Installing air..." && $(GOGET) -u github.com/cosmtrek/air)
	air

## Lint the code (requires golangci-lint)
lint:
	@which golangci-lint > /dev/null || (echo "Installing golangci-lint..." && $(GOGET) -u github.com/golangci/golangci-lint/cmd/golangci-lint)
	golangci-lint run

## Format the code
fmt:
	$(GOCMD) fmt ./...

## Vet the code
vet:
	$(GOCMD) vet ./...

## Security check (requires gosec)
security:
	@which gosec > /dev/null || (echo "Installing gosec..." && $(GOGET) github.com/securecodewarrior/gosec/v2/cmd/gosec)
	gosec ./...

## Build Docker image
docker-build:
	docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) .

## Run Docker container
docker-run:
	docker run -d --name $(DOCKER_IMAGE) --env-file .env $(DOCKER_IMAGE):$(DOCKER_TAG)

## Stop Docker container
docker-stop:
	docker stop $(DOCKER_IMAGE) || true
	docker rm $(DOCKER_IMAGE) || true

## Docker Compose up
compose-up:
	docker-compose up -d

## Docker Compose down
compose-down:
	docker-compose down

## Docker Compose logs
compose-logs:
	docker-compose logs -f

## Create .env from example
env:
	@if [ ! -f .env ]; then \
		cp .env.example .env; \
		echo ".env file created from .env.example"; \
		echo "Please edit .env file with your configuration"; \
	else \
		echo ".env file already exists"; \
	fi

## Install development tools
install-tools:
	$(GOGET) -u github.com/cosmtrek/air
	$(GOGET) -u github.com/golangci/golangci-lint/cmd/golangci-lint
	$(GOGET) -u github.com/securecodewarrior/gosec/v2/cmd/gosec

## Full development setup
setup: deps env install-tools
	@echo "Development environment setup complete!"
	@echo "1. Edit .env file with your configuration"
	@echo "2. Run 'make run' to start the application"
	@echo "3. Run 'make dev' for live reload during development"

## Release build (optimized)
release:
	CGO_ENABLED=0 $(GOBUILD) -ldflags="-w -s" -o $(BINARY_NAME) cmd/main.go

## Cross-compile for multiple platforms
cross-compile:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(BINARY_NAME)-linux-amd64 cmd/main.go
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GOBUILD) -o $(BINARY_NAME)-windows-amd64.exe cmd/main.go
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GOBUILD) -o $(BINARY_NAME)-darwin-amd64 cmd/main.go

## Show help
help:
	@echo "Available commands:"
	@echo "  build         - Build the application"
	@echo "  build-linux   - Build for Linux"
	@echo "  clean         - Clean build artifacts"
	@echo "  test          - Run tests"
	@echo "  coverage      - Run tests with coverage"
	@echo "  deps          - Download dependencies"
	@echo "  init          - Initialize Go module (creates go.sum)"
	@echo "  run           - Run the application locally"
	@echo "  dev           - Run with live reload"
	@echo "  lint          - Lint the code"
	@echo "  fmt           - Format the code"
	@echo "  vet           - Vet the code"
	@echo "  security      - Run security checks"
	@echo "  docker-build  - Build Docker image"
	@echo "  docker-run    - Run Docker container"
	@echo "  docker-stop   - Stop Docker container"
	@echo "  compose-up    - Start with Docker Compose"
	@echo "  compose-down  - Stop Docker Compose"
	@echo "  compose-logs  - View Docker Compose logs"
	@echo "  env           - Create .env from example"
	@echo "  install-tools - Install development tools"
	@echo "  setup         - Full development setup"
	@echo "  release       - Optimized release build"
	@echo "  cross-compile - Build for multiple platforms"
	@echo "  help          - Show this help"