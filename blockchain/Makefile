APP_NAME=blockchain
BUILD_DIR=bin
SOURCE_DIR=.
CONFIG_DIR=config
GOFLAGS=-ldflags="-s -w"
VERSION=1.0.0
BUILD_DATE=$(shell date +%Y-%m-%dT%H:%M:%S)
GIT_HASH=$(shell git rev-parse --short HEAD)

.PHONY: all build build-release clean run test lint migrate

all: build

build:
	@echo "Building development binary..."
	@mkdir -p $(BUILD_DIR)
	@go build $(GOFLAGS) -o $(BUILD_DIR)/$(APP_NAME) $(SOURCE_DIR)/main.go

build-release:
	@echo "Building release binary v$(VERSION)..."
	@mkdir -p $(BUILD_DIR)
	@go build $(GOFLAGS) \
		-X main.version=$(VERSION) \
		-X main.buildDate=$(BUILD_DATE) \
		-X main.gitHash=$(GIT_HASH) \
		-o $(BUILD_DIR)/$(APP_NAME) $(SOURCE_DIR)/main.go

clean:
	@echo "Cleaning build artifacts..."
	@rm -rf $(BUILD_DIR)
	@go clean -testcache

run: build
	@echo "Starting application..."
	@$(BUILD_DIR)/$(APP_NAME) --config $(CONFIG_DIR)/config.yaml

test:
	@echo "Running tests..."
	@go test -v -cover -race ./...

lint:
	@echo "Running linter..."
	@golangci-lint run

migrate:
	@echo "Running database migrations..."
	@go run migrations/main.go --config $(CONFIG_DIR)/config.yaml

install-deps:
	@echo "Installing dependencies..."
	@go get github.com/gorilla/mux
	@go get github.com/go-playground/validator/v10
	@go get github.com/go-sql-driver/mysql
	@go mod tidy

docker-build:
	@echo "Building Docker image..."
	#@docker build --no-cache -t blockchain-node:$(VERSION) .
        @docker build -t blockchain:latest .

docker-run:
	@echo "Starting Docker container..."
	@docker run -d \
		-p 8080:8080 \
		-v $(PWD)/$(CONFIG_DIR):/app/config \
		-v $(PWD)/ledger:/app/ledger \
		--name blockchain-node \
		blockchain-node:$(VERSION)
