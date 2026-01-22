# Makefile for tenant-migration tool

.PHONY: build clean

BINARY_NAME=bk-bscp-tenant-migration
BUILD_DIR=../../build/bk-bscp

build:
	@echo "Building $(BINARY_NAME)..."
	go build -o $(BUILD_DIR)/$(BINARY_NAME) .
	@echo "Build completed: $(BUILD_DIR)/$(BINARY_NAME)"

clean:
	@echo "Cleaning..."
	rm -f $(BUILD_DIR)/$(BINARY_NAME)
	@echo "Clean completed"

run:
	go run main.go

test:
	go test -v ./...

lint:
	golangci-lint run ./...
