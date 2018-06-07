VERSION := $(shell git describe --dirty=-dirty --always)

APP_NAME := atlas-db

# Absolute github repository name.
REPO := github.com/infobloxopen/atlas-db

SRC = atlas-db-controller

# Source directory path relative to $GOPATH/src.
SRCDIR = $(REPO)/$(SRC)

DEFAULT_REGISTRY := infoblox
REGISTRY ?=$(DEFAULT_REGISTRY)

IMAGE_NAME := $(REGISTRY)/$(APP_NAME):$(VERSION)
IMAGE_LATEST := $(REGISTRY)/$(APP_NAME):latest

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
test:
	echo "" > coverage.txt
	for d in `go list ./... | grep -v vendor`; do \
                t=$$(date +%s); \
                go test -v -coverprofile=cover.out -covermode=atomic $$d || exit 1; \
                echo "Coverage test $$d took $$(($$(date +%s)-t)) seconds"; \
                if [ -f cover.out ]; then \
                        cat cover.out >> coverage.txt; \
                        rm cover.out; \
                fi; \
        done

# --- Kuberenetes deployment ---
# Deploy the service in kubernetes
deploy:
	@kubectl create -f deploy/atlas-db.yaml

# Removes the kubernetes pod
remove:
	@kubectl delete -f deploy/atlas-db.yaml

vendor:
	dep update -v
