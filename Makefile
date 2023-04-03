# Ensure GOBIN is not set during build so that promu is installed to the correct path
export GOSUMDB=off
unexport GOBIN

GO           ?= go
GOFMT        ?= $(GO)fmt

pkgs          = ./...

DOCKER_REPO             ?=
DOCKER_IMAGE_NAME       ?= ephemeral-storage-exporter
DOCKER_IMAGE_TAG        ?= $(shell if [ -d .git ]; then echo `git describe --tags --dirty --always`; else echo "UNKNOWN"; fi)
DOCKERFILE_PATH         ?= ./Dockerfile

TESTFLAGS ?= -race

.PHONY: all
all: format test build

.PHONY: format
format:
	@echo ">> formatting code"
	GO111MODULE=on $(GO) mod tidy
	@$(GO) fmt $(pkgs)

.PHONY: test
test:
	@echo ">> running tests"
	@$(GO) test $(TESTFLAGS) $(pkgs)

.PHONY: build
build:
	@echo ">> building binaries"
	GO111MODULE=on $(GO) mod tidy
	GO111MODULE=on $(GO) build -o ephemeral-storage-exporter ./

.PHONY: mod-tidy
mod-tidy:
	@echo ">> checking go.mod"
	@GO111MODULE=on $(GO) mod tidy; \
	diff=$$(git diff -- go.mod go.sum 2>&1); \
	# Bring back initial state ; \
	git restore go.mod go.sum || true; \
	if [ -n "$${diff}" ]; then \
		echo "$${diff}"; \
		echo "go.mod or go.sum is not in sync with the codebase"; \
		echo "Please run 'go mod tidy' to fix this"; \
		exit 1; \
	fi

.PHONY: docker
docker: docker-build docker-push

.PHONY: docker-build
docker-build:
	@echo ">> building docker image"
	@docker build -t "$(DOCKER_REPO)/$(DOCKER_IMAGE_NAME):$(DOCKER_IMAGE_TAG)" --build-arg BIN_DIR=. .

.PHONY: docker-push
docker-push: docker-build
	@echo ">> pushing docker image"
	@docker tag $(DOCKER_REPO)/$(DOCKER_IMAGE_NAME):$(DOCKER_IMAGE_TAG) $(DOCKER_REPO)/$(DOCKER_IMAGE_NAME):latest
	@docker push "$(DOCKER_REPO)/$(DOCKER_IMAGE_NAME):$(DOCKER_IMAGE_TAG)"
	@docker push "$(DOCKER_REPO)/$(DOCKER_IMAGE_NAME):latest"
