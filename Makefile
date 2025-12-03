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

docker-build:
	docker build -t shhh:test .

docker-clean:
	@echo "Cleaning up Docker test resources..."
	@docker ps -a --filter "name=shhh" --format "{{.Names}}" | xargs -r docker rm -f 2>/dev/null || true
	@docker images --filter "reference=shhh*" --format "{{.Repository}}:{{.Tag}}" | xargs -r docker rmi -f 2>/dev/null || true
	@echo "Docker cleanup complete"

docker-clean-all: docker-clean
	@echo "Cleaning up Docker build cache..."
	@docker builder prune -f
	@echo "Docker cleanup complete (including build cache)"

.PHONY: all build clean update-htmx format-html docker-build docker-clean docker-clean-all
