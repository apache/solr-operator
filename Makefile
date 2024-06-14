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

PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
LOCALBIN = $(PROJECT_DIR)/bin

GO_VERSION = $(shell go version | sed -r 's/^.*([0-9]+\.[0-9]+\.[0-9]+).*$$/\1/g')
REQUIRED_GO_VERSION = $(shell cat go.mod | grep -E 'go [1-9]\.[0-9]+' | sed -r 's/^go ([0-9]+\.[0-9]+)$$/\1/g')

ifeq (,$(findstring $(REQUIRED_GO_VERSION),$(GO_VERSION)))
$(error Unsupported go version found $(GO_VERSION), please install go $(REQUIRED_GO_VERSION))
endif

# Image URL to use all building/pushing image targets
NAME ?= solr-operator
REPOSITORY ?= $(or $(NAMESPACE:%/=%), apache)
IMG = $(REPOSITORY)/$(NAME)
# Default tag from info in version/version.go
VERSION_SUFFIX = $(shell cat version/version.go | grep -E 'VersionSuffix([[:space:]]+)=' | sed 's/.*["'']\(.*\)["'']/\1/g')
TMP_VERSION = $(shell cat version/version.go | grep -E 'Version([[:space:]]+)=' | sed 's/.*["'"'"']\(.*\)["'"'"']/\1/g')
VERSION = $(if $(VERSION_SUFFIX),$(TMP_VERSION)-$(VERSION_SUFFIX),$(TMP_VERSION))
TAG ?= $(VERSION)
GIT_SHA = $(shell git rev-parse --short HEAD)
GOOS = $(shell go env GOOS)
ARCH = $(shell go env GOARCH)

# Default some of the testing options
TEST_PARALLELISM ?= 3

KUSTOMIZE_VERSION=v4.5.2
CONTROLLER_GEN_VERSION=v0.15.0
GO_LICENSES_VERSION=v1.6.0
GINKGO_VERSION = $(shell cat go.mod | grep 'github.com/onsi/ginkgo' | sed 's/.*\(v.*\)$$/\1/g')
KIND_VERSION=v0.23.0
YQ_VERSION=v4.33.3
CONTROLLER_RUNTIME_VERSION = $(shell cat go.mod | grep 'sigs.k8s.io/controller-runtime' | sed 's/.*\(v\(.*\)\.[^.]*\)$$/\2/g')
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION ?= 1.25.0

GO111MODULE ?= on

# Setting SHELL to bash allows bash commands to be executed by recipes.
# This is a requirement for 'setup-envtest.sh' in the test target.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: generate

.PHONY: version tag git-sha
version:
	@echo $(VERSION)

tag:
	@echo $(TAG)

git-sha:
	@echo $(GIT_SHA)

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Setup

clean: ## Clean build directories across the project
ifneq ($(wildcard $(LOCALBIN)),)
	chmod -R u+w $(LOCALBIN)
endif
	rm -rf $(LOCALBIN)
	rm -rf $(PROJECT_DIR)/testbin
	rm -rf $(PROJECT_DIR)/release-artifacts
	rm -rf $(PROJECT_DIR)/helm/*/charts $(PROJECT_DIR)/helm/*/Chart.lock
	rm -rf $(PROJECT_DIR)/cover.out
	rm -rf $(PROJECT_DIR)/generated-check

mod-tidy: ## Make sure the go mod files are up-to-date
	export GO111MODULE=on; go mod tidy

mod-clean: ## Clean up mod caches, ideally do this when upgrading go versions
	go clean -modcache


##@ Development

.PHONY: prepare
prepare: fmt generate manifests fetch-licenses-list mod-tidy ## Prepare the code for a PR or merge, should ensure that "make check" succeeds

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects
	rm -rf generated-check/api
	$(CONTROLLER_GEN) crd rbac:roleName=solr-operator-role webhook paths="./api/..." paths="./controllers/." output:rbac:artifacts:config=$(or $(TMP_CONFIG_OUTPUT_DIRECTORY),config)/rbac output:crd:artifacts:config=$(or $(TMP_CONFIG_OUTPUT_DIRECTORY),config)/crd/bases
	CONFIG_DIRECTORY=$(or $(TMP_CONFIG_OUTPUT_DIRECTORY),config) VERSION=$(VERSION) ./hack/config/add_crds_annotations.sh
	CONFIG_DIRECTORY=$(or $(TMP_CONFIG_OUTPUT_DIRECTORY),config) HELM_DIRECTORY=$(or $(TMP_HELM_OUTPUT_DIRECTORY),helm) ./hack/config/copy_crds_roles_helm.sh
	CONFIG_DIRECTORY=$(or $(TMP_CONFIG_OUTPUT_DIRECTORY),config) ./hack/config/add_crds_roles_headers.sh

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations
	$(CONTROLLER_GEN) object:headerFile="./hack/headers/header.go.txt" paths=$(or $(TMP_API_DIRECTORY),"./...")

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: fetch-licenses-list
# Ignore non-Go code warnings when it is supported natively: https://github.com/google/go-licenses/issues/120
fetch-licenses-list: mod-tidy go-licenses ## Fetch the list of license types
	$(GO_LICENSES) report . --ignore github.com/apache/solr-operator | sort > dependency_licenses.csv

.PHONY: fetch-licenses-full
fetch-licenses-full: go-licenses ## Fetch all licenses
	$(GO_LICENSES) save . --ignore github.com/apache/solr-operator --save_path licenses --force

.PHONY: build-release-artifacts
# Use path for subcommands so that we use the correct dev-dependencies rather than those installed globally
build-release-artifacts: export PATH:=$(LOCALBIN):${PATH}
build-release-artifacts: clean prepare docker-build yq kind ## Build all release artifacts for the Solr Operator
	./hack/release/artifacts/create_artifacts.sh -d $(or $(ARTIFACTS_DIR),release-artifacts) -v $(VERSION)

.PHONY: remove-version-specific-info
# Use path for subcommands so that we use the correct dev-dependencies rather than those installed globally
remove-version-specific-info: export PATH:=$(LOCALBIN):${PATH}
remove-version-specific-info: yq ## Remove information specific to a version throughout the codebase
	./hack/release/version/remove_version_specific_info.sh

.PHONY: propagate-version
# Use path for subcommands so that we use the correct dev-dependencies rather than those installed globally
propagate-version: export PATH:=$(LOCALBIN):${PATH}
propagate-version: yq ## Remove information specific to a version throughout the codebase
	./hack/release/version/propagate_version.sh

.PHONY: idea
idea: ginkgo setup-envtest ## Setup the project so to be able to run tests via IntelliJ/GoLand
	cat hack/idea/idea-setup.txt

.PHONY: kubebuilder-assets
kubebuilder-assets:
	@echo $(call kubebuilder-assets)

##@ Build

.PHONY: build
build: generate ## Build manager binary.
	GIT_SHA=${GIT_SHA} ARCH=${ARCH} GOOS=${GOOS} ./build/build.sh

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host
	go run ./main.go

##@ Docker

.PHONY: docker-build
docker-build: ## Build the docker image for the Solr Operator
	docker build --build-arg GIT_SHA=$(GIT_SHA) . -t solr-operator -f ./build/Dockerfile
	docker tag solr-operator $(IMG):$(TAG)
	docker tag solr-operator $(IMG):latest

.PHONY: docker-push
docker-push: ## Push the docker image for the Solr Operator
	docker push $(IMG):$(TAG)
	docker push $(IMG):latest

##@ Deployment

.PHONY: install
install: manifests ## Install CRDs into the K8s cluster specified in ~/.kube/config
	kubectl replace -k config/crd  2>/dev/null || kubectl create -k config/crd
	kubectl replace -f config/dependencies  2>/dev/null || kubectl create -f config/dependencies

.PHONY: uninstall
uninstall: ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config
	kubectl delete -k config/crd
	kubectl delete -f config/dependencies

.PHONY: prepare-deploy-kustomize
prepare-deploy-kustomize: kustomize
	cd config/manager && \
	cp kustomization_base.yaml kustomization.yaml && \
	$(KUSTOMIZE) edit add resource manager.yaml && \
	$(KUSTOMIZE) edit set image apache/solr-operator=${IMG}:${TAG}

.PHONY: deploy
deploy: manifests prepare-deploy-kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config
	kubectl apply -k config/default

.PHONY: undeploy
undeploy: prepare-deploy-kustomize ## Undeploy controller from the K8s cluster specified in ~/.kube/config
	kubectl delete -k config/default

##@ Tests and Checks

.PHONY: smoke-test
# Use path for subcommands so that we use the correct dev-dependencies rather than those installed globally
smoke-test: export PATH:=$(LOCALBIN):${PATH}
smoke-test: build-release-artifacts ## Run a full smoke test on a set of local release artifacts, based on the current working directory.
	./hack/release/smoke_test/smoke_test.sh \
		-l $(or $(ARTIFACTS_DIR),release-artifacts) \
		-v $(VERSION) \
		-i "${IMG}:${TAG}" \
		-s $(GIT_SHA)

.PHONY: check
check: lint test ## Do all checks, lints and tests for the Solr Operator

.PHONY: lint
lint: check-zk-op-version check-mod vet check-format check-licenses check-manifests check-generated check-helm ## Lint the codebase to check for formatting and correctness

.PHONY: check-format
check-format: ## Check the codebase to make sure it adheres to golang standards
	./hack/check_format.sh

.PHONY: check-licenses
check-licenses: go-licenses ## Ensure the licenses for dependencies are valid and the license list file is up-to-date
	@echo "Check license headers on necessary files"
	./hack/check_license.sh
	@echo "Check list of dependency licenses"
	$(GO_LICENSES) check . \
		--allowed_licenses=Apache-2.0,Apache-1.1,MIT,BSD-3-Clause,BSD-2-Clause,ISC,ICU,X11,NCSA,W3C,AFL-3.0,MS-PL,CC0-1.0,BSL-1.0,WTFPL,Unicode-DFS-2015,Unicode-DFS-2016,ZPL-2.0,UPL-1.0,Unlicense,MPL-2.0
	$(GO_LICENSES) report . --ignore github.com/apache/solr-operator 2>/dev/null | diff dependency_licenses.csv -

.PHONY: check-zk-op-version
check-zk-op-version: export PATH:=$(LOCALBIN):${PATH}
check-zk-op-version: yq ## Ensure the zookeeper-operator version is standard throughout the codebase
	./hack/zk-operator/check-version.sh

.PHONY: check-manifests
check-manifests: ## Ensure the manifest files (CRDs, RBAC, etc) are up-to-date across the project, including the helm charts
	rm -rf generated-check
	mkdir -p generated-check
	cp -r helm generated-check/helm
	cp -r config generated-check/config
	TMP_CONFIG_OUTPUT_DIRECTORY=generated-check/config TMP_HELM_OUTPUT_DIRECTORY=generated-check/helm $(MAKE) manifests
	@echo "Check to make sure the manifests are up to date"
	diff --recursive config generated-check/config
	diff --recursive helm generated-check/helm

.PHONY: check-generated
check-generated: ## Ensure the generated code is up-to-date
	rm -rf generated-check
	mkdir -p generated-check
	cp -r api generated-check/api
	TMP_API_DIRECTORY="./generated-check/api/..." $(MAKE) generate
	@echo "Check to make sure the generated code is up to date"
	diff --recursive api generated-check/api

.PHONY: check-mod
check-mod: ## Ensure the go mod files are up-to-date
	rm -rf generated-check
	mkdir -p generated-check/existing-go-mod generated-check/go-mod
	cp go.* generated-check/existing-go-mod/.
	$(MAKE) mod-tidy
	cp go.* generated-check/go-mod/.
	mv generated-check/existing-go-mod/go.* .
	@echo "Check to make sure the go mod info is up to date"
	diff go.mod generated-check/go-mod/go.mod
	diff go.sum generated-check/go-mod/go.sum

.PHONY: check-helm
check-helm: ## Ensure the helm charts lint successfully and can be read by ArtifactHub
	helm lint helm/*
	# Check that the ArtifactHub Metadata is correct
	docker run --rm --name artifact-hub-check -v "${PWD}/helm:/etc/helm" artifacthub/ah ah lint -k helm -p /etc/helm

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: check-git
check-git: ## Check to make sure the repo does not have uncommitted code
	git diff --exit-code

.PHONY: test
test: unit-tests ## Run the unit tests

.PHONY: unit-tests
unit-tests: manifests generate setup-envtest ## Run the unit tests
	# Kubebuilder-tools doesn't have a darwin+arm (i.e. Apple Silicon) distribution but the amd one works fine for our purposes
	if [[ "${GOOS}" == "darwin" && "${ARCH}" == "arm64" ]]; then export GOARCH=amd64; fi;

	KUBEBUILDER_ASSETS="$(call kubebuilder-assets)" GINKGO_EDITOR_INTEGRATION=true go test ./api/... ./controllers/... -coverprofile cover.out

.PHONY: int-tests integration-tests e2e-tests
int-tests: e2e-tests
integration-tests: e2e-tests
# Export the variables defaulted in this Makefile that are used in the e2e tests
# Variables provided by the user will automatically be passed through
e2e-tests: export OPERATOR_IMAGE=$(IMG):$(TAG)
e2e-tests: export TEST_PARALLELISM?=4
# Use path for subcommands so that we use the correct dev-dependencies rather than those installed globally
e2e-tests: export PATH:=$(LOCALBIN):${PATH}
e2e-tests: ginkgo kind manifests generate helm-dependency-build docker-build ## Run e2e/integration tests. For help, refer to: dev-docs/e2e-testing.md
	./tests/scripts/manage_e2e_tests.sh run-tests

##@ Helm

# Build the dependencies for all Helm charts
helm-dependency-build: ## Build the dependencies for all Helm charts. This will also add any necessary helm repos
	helm repo list | grep -q -w "https://charts.pravega.io" || helm repo add pravega https://charts.pravega.io
	helm dependency build helm/solr-operator
	helm dependency build helm/solr


helm-deploy-operator: helm-dependency-build docker-build ## Deploy the current version of the Solr Operator via its helm chart
	helm install solr-operator helm/solr-operator --set image.version=$(TAG) --set image.repository=$(IMG) --set image.pullPolicy=Never


##@ Dependencies
LOCALBIN ?= $(PROJECT_DIR)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

install-dependencies: controller-gen kustomize go-licenses setup-envtest kind ginkgo ## Install necessary dependencies for building and testing the Solr Operator

CONTROLLER_GEN = $(LOCALBIN)/controller-gen
.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	$(call go-get-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_GEN_VERSION))

KUSTOMIZE = $(LOCALBIN)/kustomize
.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	$(call go-get-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v4@$(KUSTOMIZE_VERSION))

GO_LICENSES = $(LOCALBIN)/go-licenses
.PHONY: go-licenses
go-licenses: $(GO_LICENSES) ## Download go-licenses locally if necessary.
$(GO_LICENSES): $(LOCALBIN)
	$(call go-get-tool,$(GO_LICENSES),github.com/google/go-licenses@$(GO_LICENSES_VERSION))

GINKGO = $(LOCALBIN)/ginkgo
.PHONY: ginkgo
ginkgo: $(GINKGO) ## Download ginkgo locally if necessary.
$(GINKGO): $(LOCALBIN)
	$(call go-get-tool,$(GINKGO),github.com/onsi/ginkgo/v2/ginkgo@$(GINKGO_VERSION))

KIND = $(LOCALBIN)/kind
.PHONY: kind
kind: $(KIND) ## Download kind locally if necessary.
$(KIND): $(LOCALBIN)
	$(call go-get-tool,$(KIND),sigs.k8s.io/kind@$(KIND_VERSION))

YQ = $(LOCALBIN)/yq
.PHONY: yq
yq: $(YQ) ## Download yq locally if necessary.
$(YQ): $(LOCALBIN)
	$(call go-get-tool,$(YQ),github.com/mikefarah/yq/v4@$(YQ_VERSION))

SETUP_ENVTEST = $(LOCALBIN)/setup-envtest
.PHONY: setup-envtest
setup-envtest: $(SETUP_ENVTEST) ## Download setup-envtest locally if necessary.
$(SETUP_ENVTEST): $(LOCALBIN)
	$(call go-get-tool,$(SETUP_ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest@release-$(CONTROLLER_RUNTIME_VERSION))

# go-get-tool will 'go get' any package $2 and install it to $1.
define go-get-tool
@[ -f $(1) ] || { \
set -e ;\
echo "Downloading $(2)" ;\
GOBIN=$(LOCALBIN) go install $(2) ;\
}
endef

# go-get-tool will 'go get' any package $2 and install it to $1.
define kubebuilder-assets
$(shell $(SETUP_ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)
endef
