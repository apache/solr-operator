# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd"

# Image URL to use all building/pushing image targets
NAME ?= solr-operator
NAMESPACE ?= apache/
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

release: clean manifests lint
	VERSION=${VERSION} bash hack/release/update_versions.sh
	VERSION=${VERSION} bash hack/release/build_helm.sh
	VERSION=${VERSION} bash hack/release/setup_release.sh

###
# Building
###

# Build solr-operator binary
build: generate vet
	BIN=solr-operator VERSION=${VERSION} GIT_SHA=${GIT_SHA} ARCH=${ARCH} GOOS=${GOOS} ./build/build.sh

# Run against the configured Kubernetes cluster in ~/.kube/config
run: generate fmt vet manifests
	go run ./main.go

# Install CRDs into a cluster
install: manifests
	kubectl replace -k config/crd || kubectl create -k config/crd

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: manifests install
	cd config/manager && touch kustomization.yaml && kustomize edit add resource manager.yaml && kustomize edit set image apache/solr-operator=${IMG}:${VERSION}
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

# Run go vet against code
fetch-licenses-list:
	go-licenses csv . | sort > dependency_licenses.csv

# Run go vet against code
fetch-licenses-full:
	go-licenses save . --save_path licenses --force

###
# Tests and Checks
###

check: lint test

lint: check-format check-licenses check-manifests check-generated check-helm

check-format:
	./hack/check_format.sh

check-licenses:
	@echo "Check license headers on necessary files"
	./hack/check_license.sh
	@echo "Check list of dependency licenses"
	go-licenses csv . 2>/dev/null | sort | diff dependency_licenses.csv -

check-manifests: manifests
	@echo "Check to make sure the manifests are up to date"
	git diff --exit-code -- config helm/solr-operator/crds

check-generated: generate
	@echo "Check to make sure the generated code is up to date"
	git diff --exit-code -- api/*/zz_generated.deepcopy.go

check-helm:
	helm lint helm/solr-operator

# Run tests
test: generate vet manifests
	go test ./... -coverprofile cover.out


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

# Build the docker image for the operator
docker-build:
	docker build --build-arg VERSION=$(VERSION) --build-arg GIT_SHA=$(GIT_SHA) . -t solr-operator -f ./build/Dockerfile
	docker tag solr-operator ${IMG}:${VERSION}
	docker tag solr-operator ${IMG}:latest

# Push the docker image for the operator
docker-push:
	docker push ${IMG}:${VERSION}
	docker push ${IMG}:latest
