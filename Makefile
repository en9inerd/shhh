GO=go
DIST_DIR=dist
BINARY_NAME=$(shell basename $(PWD))
BINARY_PATH=$(DIST_DIR)/$(BINARY_NAME)

# Targets
all: build

build:
	$(GO) build -o $(BINARY_PATH) ./cmd/shhh/

build-prod:
	bash scripts/build.sh

clean:
	rm -rf $(DIST_DIR)

update-htmx:
	./scripts/update-htmx.sh

format-html:
	npx --yes prettier --write "ui/templates/**/*.html"

.PHONY: all build clean update-htmx format-html
