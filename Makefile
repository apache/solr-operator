# Licensed to the Apache Software Foundation (ASF) under one or more
# contributor license agreements.  See the NOTICE file distributed with
# this work for additional information regarding copyright ownership.
# The ASF licenses this file to You under the Apache License, Version 2.0
# (the "License"); you may not use this file except in compliance with
# the License.  You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:trivialVersions=true"

# Image URL to use all building/pushing image targets
NAME ?= solr-operator
REPOSITORY ?= $(or $(NAMESPACE:%/=%), apache)
IMG = $(REPOSITORY)/$(NAME)
# Default tag from info in version/version.go
VERSION_SUFFIX = $(shell cat version/version.go | grep -E 'VersionSuffix([[:space:]]+)string' | grep -o '["''].*["'']' | xargs)
TMP_VERSION = $(shell cat version/version.go | grep -E 'Version([[:space:]]+)string' | grep -o '["''].*["'']' | xargs)
ifneq (,$(VERSION_SUFFIX))
VERSION = $(TMP_VERSION)-$(VERSION_SUFFIX)
else
VERSION = ${TMP_VERSION}
endif
TAG ?= $(VERSION)
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

.PHONY: version tag
version:
	@echo $(VERSION)

tag:
	@echo $(TAG)

###
# Setup
###

clean:
	rm -rf ./bin
	rm -rf ./release-artifacts
	rm -rf ./helm/solr-operator/charts ./helm/solr-operator/Chart.lock
	rm -rf ./cover.out
	rm -rf ./generated-check

mod-tidy:
	export GO111MODULE=on; go mod tidy

.install-dependencies:
	. ./hack/install_dependencies.sh

install-dependencies: .install-dependencies mod-tidy

build-release-artifacts: clean prepare docker-build
	./hack/release/artifacts/create_artifacts.sh -d $(or $(ARTIFACTS_DIR),release-artifacts)

###
# Building
###

# Prepare the code for a PR or merge
prepare: fmt generate manifests fetch-licenses-list mod-tidy

# Build solr-operator binary
build: generate
	BIN=solr-operator GIT_SHA=${GIT_SHA} ARCH=${ARCH} GOOS=${GOOS} ./build/build.sh

# Run against the configured Kubernetes cluster in ~/.kube/config
run: generate fmt manifests
	go run ./main.go

# Install CRDs into a cluster
install: manifests
	kubectl replace -k config/crd || kubectl create -k config/crd

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: manifests install
	cd config/manager && touch kustomization.yaml && kustomize edit add resource manager.yaml && kustomize edit set image apache/solr-operator=${IMG}:${TAG}
	kubectl apply -k config/default

# Generate code
generate:
	controller-gen object:headerFile=./hack/headers/header.go.txt paths="./..."

# Generate manifests e.g. CRD, RBAC etc.
manifests:
	rm -r config/crd/bases
	controller-gen $(CRD_OPTIONS) rbac:roleName=solr-operator-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases
	./hack/config/copy_crds_roles_helm.sh
	./hack/config/add_crds_roles_headers.sh

# Run go fmt against code
fmt:
	go fmt ./...

# Fetch the list of license types
fetch-licenses-list:
	go-licenses csv . | grep -v -E "solr-operator" | sort > dependency_licenses.csv

# Fetch all licenses
fetch-licenses-full:
	go-licenses save . --save_path licenses --force

###
# Tests and Checks
###

check: lint test

lint: check-mod vet check-format check-licenses check-manifests check-generated check-helm

check-format:
	./hack/check_format.sh

check-licenses:
	@echo "Check license headers on necessary files"
	./hack/check_license.sh
	@echo "Check list of dependency licenses"
	go-licenses csv . 2>/dev/null | grep -v -E "solr-operator" | sort | diff dependency_licenses.csv -

check-manifests:
	rm -rf generated-check
	mkdir -p generated-check
	mv config generated-check/existing-config; cp -r generated-check/existing-config config
	mv helm generated-check/existing-helm; cp -r generated-check/existing-helm helm
	make manifests
	mv config generated-check/config; mv generated-check/existing-config config
	mv helm generated-check/helm; mv generated-check/existing-helm helm
	@echo "Check to make sure the manifests are up to date"
	diff --recursive config generated-check/config
	diff --recursive helm generated-check/helm

check-generated:
	rm -rf generated-check
	mkdir -p generated-check
	cp -r api generated-check/existing-api
	make generate
	mv api generated-check/api; mv generated-check/existing-api api
	@echo "Check to make sure the generated code is up to date"
	diff --recursive api generated-check/api

check-helm:
	helm lint helm/solr-operator

check-mod:
	rm -rf generated-check
	mkdir -p generated-check/existing-go-mod generated-check/go-mod
	cp go.* generated-check/existing-go-mod/.
	make mod-tidy
	cp go.* generated-check/go-mod/.
	mv generated-check/existing-go-mod/go.* .
	@echo "Check to make sure the go mod info is up to date"
	diff go.mod generated-check/go-mod/go.mod
	diff go.sum generated-check/go-mod/go.sum

# Run go vet against code
vet:
	go vet ./...

check-git:
	@echo "Check to make sure the repo does not have uncommitted code"
	git diff --exit-code

# Run tests
test:
	go test ./... -coverprofile cover.out


###
# Helm
###

# Build the docker image for the operator
helm-dependency-build:
	helm dependency build helm/solr-operator

# Push the docker image for the operator
helm-deploy-operator: helm-dependency-build docker-build
	helm install solr-operator helm/solr-operator --set image.version=$(TAG) --set image.repository=$(REPOSITORY)


###
# Docker Building & Pushing
###

# Build the docker image for the operator
docker-build:
	docker build --build-arg GIT_SHA=$(GIT_SHA) . -t solr-operator -f ./build/Dockerfile
	docker tag solr-operator ${IMG}:${TAG}
	docker tag solr-operator ${IMG}:latest

# Push the docker image for the operator
docker-push:
	docker push ${IMG}:${TAG}
	docker push ${IMG}:latest
