OS ?= $(shell uname -s | tr '[:upper:]' '[:lower:]')
ARCH ?= $(shell uname -m | sed 's/x86_64/amd64/')
PACKAGE_NAME ?= image-updater
BUILD_DIR ?= dist
BUILD_TARGET = $(BUILD_DIR)/$(PACKAGE_NAME)-$(OS)-$(ARCH)
BUILD_DEPS = $(wildcard **/*.go) go.mod go.sum
PACKAGE_TARGET = $(BUILD_DIR)/$(PACKAGE_NAME)-$(OS)-$(ARCH).tar.gz
PACKAGE_DEPS = $(BUILD_TARGET)
ARGS ?= ""

all: test build

build: $(BUILD_TARGET)

package: $(PACKAGE_TARGET)

$(PACKAGE_TARGET): $(PACKAGE_DEPS)
	@tar -czf $(PACKAGE_TARGET) -C $(BUILD_DIR) $(PACKAGE_NAME)-$(OS)-$(ARCH) --transform 's/$(PACKAGE_NAME)-$(OS)-$(ARCH)/$(PACKAGE_NAME)/'

$(BUILD_TARGET): $(BUILD_DEPS)
	@go build -o $(BUILD_TARGET) -v

test:
	@go test -v ./...

clean:
	@rm -rf $(BUILD_DIR)
	@go clean

run: $(BUILD_TARGET)
	@$(BUILD_TARGET) $(ARGS)

gen: $(BUILD_TARGET)
	@$(BUILD_TARGET) gen .

check:
	@go fmt ./...

fmt:
	@go fmt ./...

.PHONY: all build package test clean run gen check fmt
