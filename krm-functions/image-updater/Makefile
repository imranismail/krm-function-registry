OS ?= $(shell uname -s | tr '[:upper:]' '[:lower:]')
ARCH ?= $(shell uname -m | sed 's/x86_64/amd64/')
PACKAGE_NAME ?= image-updater
BUILD_DIR ?= dist
ARGS ?= ""

all: test build

build: $(BUILD_DIR)/$(PACKAGE_NAME)

package:$(BUILD_DIR)/$(PACKAGE_NAME)-$(OS)-$(ARCH).tar.gz

$(BUILD_DIR)/$(PACKAGE_NAME)-$(OS)-$(ARCH).tar.gz: $(BUILD_DIR)/$(PACKAGE_NAME)
	@tar -czf $(BUILD_DIR)/$(PACKAGE_NAME)-$(OS)-$(ARCH).tar.gz -C $(BUILD_DIR) $(PACKAGE_NAME)

$(BUILD_DIR)/$(PACKAGE_NAME): $(wildcard **/*.go) go.mod go.sum
	@go build -o $(BUILD_DIR)/$(PACKAGE_NAME) -v

test:
	@go test -v ./...

clean:
	@rm -rf $(BUILD_DIR)
	@go clean

run: build
	@$(BUILD_DIR)/$(PACKAGE_NAME) $(ARGS)

gen: build
	@$(BUILD_DIR)/$(PACKAGE_NAME) gen .

check:
	@go fmt ./...

fmt:
	@go fmt ./...

.PHONY: all test clean run
