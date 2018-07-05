REPO              := github.com/infobloxopen/atlas-db
BUILD_PATH        := bin

# configuration for image
APP_NAME          := atlas-db
DEFAULT_REGISTRY  := infoblox
REGISTRY          ?= $(DEFAULT_REGISTRY)
DEFAULT_VERSION   := $(shell git describe --dirty=-dirty --always)
VERSION           ?= $(DEFAULT_VERSION)
IMAGE_NAME        := $(REGISTRY)/$(APP_NAME):$(VERSION)
IMAGE_LATEST      := $(REGISTRY)/$(APP_NAME):latest

# configuration for source
SRC               = atlas-db-controller
SRCDIR            = $(REPO)/$(SRC)

# configuration for building

BUILD_TYPE ?= "default"
ifeq ($(BUILD_TYPE), "default")
	GO_PATH              	:= /go
	SRCROOT_ON_HOST      	:= $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
	SRCROOT_IN_CONTAINER	:= $(GO_PATH)/src/$(REPO)
	GO_CACHE             	:= -pkgdir $(SRCROOT_IN_CONTAINER)/$(BUILD_PATH)/go-cache

	DOCKER_RUNNER        	:= docker run --rm
	DOCKER_RUNNER        	+= -v $(SRCROOT_ON_HOST):$(SRCROOT_IN_CONTAINER)
	DOCKER_BUILDER       	:= infoblox/buildtool:v8
	BUILDER              	:= $(DOCKER_RUNNER) -w $(SRCROOT_IN_CONTAINER) $(DOCKER_BUILDER)
endif

GO_BUILD_FLAGS		?= $(GO_CACHE) -i -v
GO_TEST_FLAGS			?= -v -cover
# TEMPORARY FIX
IGNORE_FAKE       := grep -v fake
GO_TEST_PACKAGES	:= $(shell $(BUILDER) go list ./... | grep -v "./vendor/"|$(IGNORE_FAKE))
SEARCH_GOFILES		:= $(BUILDER) find . -not -path '*/vendor/*' -type f -name "*.go"

.PHONY: default
default: build

# formats the repo
fmt:
	@echo "Running 'go fmt ...'"
	@$(SEARCH_GOFILES) -exec gofmt -s -w {} \;

deps:
	@echo "Getting dependencies..."
	$(BUILDER) dep ensure

clean:
	@docker rmi "$(IMAGE_NAME)"
	@docker rmi "$(IMAGE_LATEST)"

# Builds the docker image
build: deps fmt
	@docker build --no-cache -t $(IMAGE_NAME) -f Dockerfile .
	@docker tag $(IMAGE_NAME) $(IMAGE_LATEST)

# Pushes the image to docker
push: build
	@docker push $(IMAGE_NAME)
	@docker push $(IMAGE_LATEST)

# Runs the tests
test: check-fmt
	@echo "Running test cases..."
	@$(BUILDER) go test $(GO_TEST_FLAGS) $(GO_TEST_PACKAGES)

check-fmt:
	@echo "Checking go formatting..."
	@test -z `go fmt $(GO_TEST_PACKAGES)`

# --- Kuberenetes deployment ---
# Deploy the service in kubernetes
deploy:
	@kubectl create -f deploy/atlas-db.yaml

# Removes the kubernetes pod
remove:
	@kubectl delete -f deploy/atlas-db.yaml

vendor:
	$(BUILDER) dep update -v
