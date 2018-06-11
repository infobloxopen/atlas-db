# Absolute github repository name.
REPO              := github.com/infobloxopen/atlas-db

# Utility docker image to generate Go files from .proto definition.
# https://github.com/infobloxopen/buildtool
BUILDTOOL_IMAGE   := infoblox/buildtool:v2

# configuration for image
APP_NAME          := atlas-db
DEFAULT_REGISTRY  := infoblox
REGISTRY          ?=$(DEFAULT_REGISTRY)
VERSION           := $(shell git describe --dirty=-dirty --always)
IMAGE_NAME        := $(REGISTRY)/$(APP_NAME):$(VERSION)
IMAGE_LATEST      := $(REGISTRY)/$(APP_NAME):latest

# configuration for source
SRC               = atlas-db-controller
SRCDIR            = $(REPO)/$(SRC)

# configuration for output
BIN               = atlas-db-controller
BINDIR            = $(CURDIR)/bin

# configuration for building
GO_TEST_FLAGS     ?= -v -cover
# TEMPORARY FIX
IGNORE_FAKE       := grep -v fake
GO_PACKAGES       := $(shell go list ./... | grep -v vendor | $(IGNORE_FAKE))

.PHONY: default
default: build

build: fmt bin
	GOOS=linux go build -o "$(BINDIR)/$(BIN)" "$(SRCDIR)"

fmt:
	@echo "Running 'go fmt ...'"
	@go fmt -x "$(REPO)/..."

deps:
	@echo "Getting dependencies..."
	@dep ensure

bin:
	mkdir -p "$(BINDIR)"

clean:
	@rm -rf "$(BINDIR)"
	@rm -rf .glide

# --- Docker commands ---
# Builds the docker image
image:
	@docker build -t $(IMAGE_NAME) -f docker/Dockerfile .
	@docker tag $(IMAGE_NAME) $(IMAGE_LATEST)

# Pushes the image to docker
push: image
	@docker push $(IMAGE_NAME)
	@docker push $(IMAGE_LATEST)

# Runs the tests
test: check-fmt
	@echo "Running test cases..."
	@go test $(GO_TEST_FLAGS) $(GO_PACKAGES)

check-fmt:
	@echo "Checking go formatting..."
	@test -z `go fmt $(GO_PACKAGES)`

# --- Kuberenetes deployment ---
# Deploy the service in kubernetes
deploy:
	@kubectl create -f deploy/atlas-db.yaml

# Removes the kubernetes pod
remove:
	@kubectl delete -f deploy/atlas-db.yaml

vendor:
	dep update -v
