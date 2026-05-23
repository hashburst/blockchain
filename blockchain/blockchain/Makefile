APP_NAME=blockchain
BUILD_DIR=bin
SOURCE_DIR=.
GOFLAGS=-ldflags="-s -w"

.PHONY: all build clean run test

all: build

build:
	@echo "Building blockchain application..."
	@mkdir -p $(BUILD_DIR)
	@go build $(GOFLAGS) -o $(BUILD_DIR)/$(APP_NAME) $(SOURCE_DIR)/main.go

clean:
	@echo "Cleaning build artifacts..."
	@rm -rf $(BUILD_DIR)
	@go clean -testcache

run: build
	@echo "Starting blockchain node..."
	@$(BUILD_DIR)/$(APP_NAME)

test:
	@echo "Running tests..."
	@go test -v -cover ./...

deps:
	@echo "Installing dependencies..."
	@go get github.com/gorilla/mux
	@go get gopkg.in/yaml.v2
	@go get github.com/go-sql-driver/mysql
	@go mod tidy
