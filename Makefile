REPO              := github.com/infobloxopen/atlas-db

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

# configuration for building
GO_TEST_FLAGS     ?= -v -cover
# TEMPORARY FIX
IGNORE_FAKE       := grep -v fake
GO_PACKAGES       := $(shell go list ./... | grep -v vendor | $(IGNORE_FAKE))

.PHONY: default
default: build

# formats the repo
fmt:
	@echo "Running 'go fmt ...'"
	@go fmt -x "$(REPO)/..."

deps:
	@echo "Getting dependencies..."
	@dep ensure

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
