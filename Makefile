GO=go
DIST_DIR=dist
BINARY_NAME=$(shell basename $(PWD))
BINARY_PATH=$(DIST_DIR)/$(BINARY_NAME)

# Targets
all: build

build:
	$(GO) build -o $(BINARY_PATH) ./cmd/app/

build-prod:
	bash scripts/build.sh

clean:
	rm -rf $(DIST_DIR)

.PHONY: all build clean
