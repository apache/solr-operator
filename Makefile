# Image URL to use all building/pushing image targets
NAME ?= solr-operator
NAMESPACE ?= bloomberg/
IMG = $(NAMESPACE)$(NAME)
VERSION ?= 0.1.3
GIT_SHA = $(shell git rev-parse --short HEAD)
GOOS = $(shell go env GOOS)
ARCH = $(shell go env GOARCH)

all: generate

version:
	@echo $(VERSION)

###
# Setup
###

clean:
	rm -rf ./vendor
	rm -rf ./bin

vendor:
	rm -rf ./vendor
	dep ensure -v -vendor-only

###
# Building
###

# Build manager binary
build: generate vet
	BIN=manager VERSION=${VERSION} GIT_SHA=${GIT_SHA} ARCH=${ARCH} GOOS=${GOOS} ./build/build.sh

# Generate manifests e.g. CRD, RBAC etc.
manifests: generate
	go run vendor/sigs.k8s.io/controller-tools/cmd/controller-gen/main.go all
	rm -r config/default/rbac
	mv config/rbac config/default/
	@echo "updating kustomize image patch file for manager resource"
	sed -i.bak -e 's@image: .*@image: '"${IMG}:${VERSION}"'@' ./config/default/manager_image_patch.yaml
	rm ./config/default/manager_image_patch.yaml.bak
	kustomize build config/default > config/operators/solr_operator.yaml

# Generate code
generate:
ifndef GOPATH
	$(error GOPATH not defined, please define GOPATH. Run "go help gopath" to learn more about GOPATH)
endif
	#./hack/update-codegen.sh
	go generate ./pkg/... ./cmd/...

###
# Testing
###

# Run tests
test: check-format check-license vet
	# ./hack/verify-codegen.sh
	go test ./pkg/... ./cmd/... -coverprofile cover.out

# Run go vet against code
vet:
	go vet ./pkg/... ./cmd/...

check-format:
	./hack/check_format.sh

check-license:
	./hack/check_license.sh

manifests-check:
	@echo "Check to make sure the manifests are up to date"
	git diff --exit-code -- config

###
# Running the operator
###

# Run against the configured Kubernetes cluster in ~/.kube/config
run: generate vet
	go run ./cmd/manager/main.go

# Install CRDs into a cluster
install: manifests
	kubectl apply -f config/crds

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: manifests
	kubectl apply -f config/crds
	kubectl apply -f config/operators/solr_operator.yaml

###
# Docker Building & Pushing
###

# Build the base builder docker image
# This can be a static go build or dynamic
docker-base-build:
	docker build --build-arg VERSION=$(VERSION) --build-arg GIT_SHA=$(GIT_SHA) . -t solr-operator-build -f ./build/Dockerfile.build.dynamic

# Build the docker image for the operator only
docker-build:
	docker build --build-arg BUILD_IMG=solr-operator-build . -t solr-operator -f ./build/Dockerfile.slim
	docker tag solr-operator ${IMG}:${VERSION}
	docker tag solr-operator ${IMG}:latest

# Build the docker image for the operator, containing the vendor deps as well
docker-vendor-build:
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
