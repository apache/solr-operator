# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd"

# Image URL to use all building/pushing image targets
NAME ?= solr-operator
NAMESPACE ?= bloomberg/
IMG = $(NAMESPACE)$(NAME)
VERSION ?= 0.1.4
GIT_SHA = $(shell git rev-parse --short HEAD)
GOOS = $(shell go env GOOS)
ARCH = $(shell go env GOARCH)
GO111MODULE ?= on
# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

all: generate

version:
	@echo $(VERSION)

###
# Setup
###

clean:
	rm -rf ./bin

vendor:
	export GO111MODULE=on; go mod tidy

###
# Building
###

# Build manager binary
build: generate vet
	BIN=manager VERSION=${VERSION} GIT_SHA=${GIT_SHA} ARCH=${ARCH} GOOS=${GOOS} ./build/build.sh

# Run tests
test: check-format check-license generate fmt vet manifests
	go test ./... -coverprofile cover.out

# Build manager binary
manager: generate fmt vet
	go build -o bin/manager main.go

# Run against the configured Kubernetes cluster in ~/.kube/config
run: generate fmt vet manifests
	go run ./main.go

# Install CRDs into a cluster
install: manifests
	kustomize build config/crd | kubectl apply -f -

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: manifests
	cd config/manager && kustomize edit set image controller=${IMG}
	kustomize build config/default | kubectl apply -f -

# Generate manifests e.g. CRD, RBAC etc.
manifests: controller-gen
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=solr-operator-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases

# Run go fmt against code
fmt:
	go fmt ./...

# Run go vet against code
vet:
	go vet ./...

check-format:
	./hack/check_format.sh

check-license:
	./hack/check_license.sh

manifests-check:
	@echo "Check to make sure the manifests are up to date"
	git diff --exit-code -- config

# Generate code
generate: controller-gen
	$(CONTROLLER_GEN) object:headerFile=./hack/boilerplate.go.txt paths="./..."


docker-build: test
	docker build . -t ${IMG}:${VERSION}

# Push the docker image for the operator
docker-push:
	docker push ${IMG}:${VERSION}
	docker push ${IMG}:latest

# # find or download controller-gen
# download controller-gen if necessary
controller-gen:
ifeq (, $(shell which controller-gen))
    go get sigs.k8s.io/controller-tools/cmd/controller-gen@v0.2.2
CONTROLLER_GEN=$(shell go env GOPATH)/bin/controller-gen
else
CONTROLLER_GEN=$(shell which controller-gen)
endif
