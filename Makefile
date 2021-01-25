# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd"

# Image URL to use all building/pushing image targets
NAME ?= solr-operator
NAMESPACE ?= bloomberg/
IMG = $(NAMESPACE)$(NAME)
VERSION ?= $(or $(shell git describe --tags HEAD),"latest")
GIT_SHA = $(shell git rev-parse --short HEAD)
GOOS = $(shell go env GOOS)
ARCH = $(shell go env GOARCH)
CONTROLLER_GEN_VERSION=v0.4.1
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
	rm -rf ./release-artifacts

mod-tidy:
	export GO111MODULE=on; go mod tidy

release: clean manifests helm-check
	VERSION=${VERSION} bash hack/release/update_versions.sh
	VERSION=${VERSION} bash hack/release/build_helm.sh
	VERSION=${VERSION} bash hack/release/setup_release.sh

###
# Building
###

# Build solr-operator binary
build: generate vet
	BIN=solr-operator VERSION=${VERSION} GIT_SHA=${GIT_SHA} ARCH=${ARCH} GOOS=${GOOS} ./build/build.sh

# Run tests
test: check-format check-license generate fmt vet manifests
	go test ./... -coverprofile cover.out

# Run against the configured Kubernetes cluster in ~/.kube/config
run: generate fmt vet manifests
	go run ./main.go

# Install CRDs into a cluster
install: manifests
	kubectl replace -k config/crd || kubectl create -k config/crd

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: manifests install
	cd config/manager && touch kustomization.yaml && kustomize edit add resource manager.yaml && kustomize edit set image bloomberg/solr-operator=${IMG}:${VERSION}
	kubectl apply -k config/default

# Generate code
generate: controller-gen mod-tidy
	$(CONTROLLER_GEN) object:headerFile=./hack/headers/header.go.txt paths="./..."

# Generate manifests e.g. CRD, RBAC etc.
manifests: controller-gen mod-tidy
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=solr-operator-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases
	./hack/helm/copy_crds_roles_helm.sh

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

helm-check:
	helm lint helm/solr-operator


# # find or download controller-gen
# download controller-gen if necessary
controller-gen:
ifeq (, $(shell which controller-gen))
	go get "sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_GEN_VERSION)"
else ifneq (Version: $(CONTROLLER_GEN_VERSION), $(shell controller-gen --version))
	rm "$(shell which controller-gen)"
	go get "sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_GEN_VERSION)"
endif
CONTROLLER_GEN=$(shell which controller-gen)


###
# Docker Building & Pushing
###

# Build the base builder docker image
# This can be a static go build or dynamic
docker-base-build:
	docker build --build-arg VERSION=$(VERSION) --build-arg GIT_SHA=$(GIT_SHA) . -t solr-operator-build -f ./build/Dockerfile.build

# Build the docker image for the operator only
docker-build: docker-base-build
	docker build --build-arg BUILD_IMG=solr-operator-build . -t solr-operator -f ./build/Dockerfile.slim
	docker tag solr-operator ${IMG}:${VERSION}
	docker tag solr-operator ${IMG}:latest

# Build the docker image for the operator, containing the vendor deps as well
docker-vendor-build: docker-build
	docker build --build-arg BUILD_IMG=solr-operator-build --build-arg SLIM_IMAGE=solr-operator . -t solr-operator-vendor -f ./build/Dockerfile.vendor
	docker tag solr-operator-vendor ${IMG}:${VERSION}-vendor
	docker tag solr-operator-vendor ${IMG}:latest-vendor

# Push the docker image for the operator
docker-push:
	docker push ${IMG}:${VERSION}
	docker push ${IMG}:latest

# Push the docker image for the operator with vendor deps
docker-vendor-push:
	docker push ${IMG}:${VERSION}-vendor
	docker push ${IMG}:latest-vendor
