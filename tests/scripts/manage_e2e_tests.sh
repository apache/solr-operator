#!/usr/bin/env bash
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

# exit immediately when a command fails
set -e
# only exit with zero if all commands of the pipeline exit successfully
set -o pipefail
# error on unset variables
set -u

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
REPO_DIR=$( cd -- "${SCRIPT_DIR}" &> /dev/null && cd -- ../.. &> /dev/null && pwd )

show_help() {
cat << EOF
Usage: ./tests/scripts/manage_e2e_tests.sh (run-tests|create-cluster|destroy-cluster|kubeconfig) [-h] [-k KUBERNETES_VERSION] [-s SOLR_IMAGE] [-a ADDITIONAL_IMAGES (bash array)] -i OPERATOR_IMAGE

Manage the Solr Operator E2E tests via a KinD cluster.

Available actions are: run-tests, create-cluster, destroy-cluster, kubeconfig

    -h  Display this help and exit
    -i  Solr Operator docker image to use (Optional, defaults to apache/solr-operator:<version>)
    -k  Kubernetes Version to test with (full tag, e.g. v1.24.16) (Optional, defaults to a compatible version)
    -s  Full solr image, or image tag (for the official Solr image), to test with (e.g. apache/solr-nightly:9.0.0, 8.11). (Optional, defaults to a compatible version)
    -a  Load additional local images into the test Kubernetes cluster. Provide option multiple times for multiple images. (Optional)
EOF
}

ADDITIONAL_IMAGES=("pravega/zookeeper:0.2.15")
OPTIND=1
while getopts hv:i:k:s:a: opt; do
    case $opt in
        h)
            show_help
            exit 0
            ;;
        i)  OPERATOR_IMAGE=$OPTARG
            ;;
        k)  KUBERNETES_VERSION=$OPTARG
            ;;
        s)  SOLR_IMAGE=$OPTARG
            ;;
        a)  ADDITIONAL_IMAGES+=("$OPTARG")
            ;;
        *)
            show_help >&2
            exit 1
            ;;
    esac
done
shift "$((OPTIND-1))"   # Discard the options and sentinel --
ACTION="${1:-}"
if [[ -z "${ACTION:-}" ]]; then
  echo "You must specify a cluster action. Either 'run-tests', 'create-cluster', 'destroy-cluster' or 'kubeconfig'." >&2 && exit 1
fi
ACTION_ARGS=("${@:2}")

if [[ -z "${OPERATOR_IMAGE:-}" ]]; then
  echo "Specify a Docker image for the Solr Operator through -i, or through the OPERATOR_IMAGE env var" >&2 && exit 1
fi
if [[ -z "${KUBERNETES_VERSION:-}" ]]; then
  KUBERNETES_VERSION="v1.24.15"
fi
if [[ -z "${SOLR_IMAGE:-}" ]]; then
  SOLR_IMAGE="${SOLR_VERSION:-8.11}"
fi
if [[ "${SOLR_IMAGE}" != *":"* ]]; then
  SOLR_IMAGE="solr:${SOLR_IMAGE}"
fi
IFS=$'\036'; RAW_GINKGO=(${RAW_GINKGO:-}); unset IFS

CLUSTER_NAME="$(echo "solr-op-e2e-${OPERATOR_IMAGE##*:}-k-${KUBERNETES_VERSION}-s-${SOLR_IMAGE##*:}"  | tr '[:upper:]' '[:lower:]' | sed "s/snapshot/snap/" | sed "s/prerelease/pre/")"
export CLUSTER_NAME
export KUBE_CONTEXT="kind-${CLUSTER_NAME}"
export KUBERNETES_VERSION
export OPERATOR_IMAGE
export SOLR_IMAGE
export ADDITIONAL_IMAGES
export RAW_GINKGO

# Cluster Operation Options
export REUSE_KIND_CLUSTER_IF_EXISTS="${REUSE_KIND_CLUSTER_IF_EXISTS:-true}" # This is used for all start_cluster calls
export LEAVE_KIND_CLUSTER_ON_SUCCESS="${LEAVE_KIND_CLUSTER_ON_SUCCESS:-false}" # This is only used when using run_tests or run_with_cluster

function add_image_to_kind_repo_if_local() {
  IMAGE="$1"
  PULL_IF_NOT_LOCAL="$2"
  if (docker image inspect "${IMAGE}" &>/dev/null); then
    printf "\nUsing local version of image \"%s\".\nIf you want to use an updated version of this image, run \"docker pull %s\" before running the integration tests again.\n\n" "${IMAGE}" "${IMAGE}"
    kind load docker-image --name "${CLUSTER_NAME}" "${IMAGE}"
  else
    if [ "${PULL_IF_NOT_LOCAL}" = true ]; then
      printf "\nPulling image \"%s\" since it was not found locally.\n\n" "${IMAGE}"
      docker pull "${IMAGE}"
      kind load docker-image --name "${CLUSTER_NAME}" "${IMAGE}"
    else
      printf "\nUsing the remote image \"%s\", since it was not found in the local Docker image list.\n\n" "${IMAGE}"
    fi
  fi
}

# These gingko params are customized via the following envVars
GINKGO_PARAM_NAMES=(--seed    --procs          --focus-file --label-filter --focus      --skip)
GINKGO_PARAM_ENVS=( TEST_SEED TEST_PARALLELISM TEST_FILES   TEST_LABELS    TEST_FILTER TEST_SKIP)
function run_tests() {
  start_cluster

  GINKGO_PARAMS=()
  for idx in "${!GINKGO_PARAM_NAMES[@]}"; do
    param=${GINKGO_PARAM_NAMES[$idx]}
    envName=${GINKGO_PARAM_ENVS[$idx]}
    if [[ -n "${!envName:-}" ]]; then
      GINKGO_PARAMS+=("${param}" "${!envName}")
    fi
  done
  GINKGO_PARAMS+=("${RAW_GINKGO[@]}")

  GINKGO_EDITOR_INTEGRATION=true ginkgo --randomize-all "${GINKGO_PARAMS[@]}" "${REPO_DIR}"/tests/e2e/...

  printf "\n********************\n"
  printf "Local end-to-end cluster test successfully run!\n\n"

  if [[ "${LEAVE_KIND_CLUSTER_ON_SUCCESS}" != true ]]; then
    printf "Deleting test KinD Kubernetes cluster after a successful test run.\n\n"
    delete_cluster
  fi
}

function export_kubeconfig() {
  kind export kubeconfig --name "${CLUSTER_NAME}"
}

function delete_cluster() {
  kind delete clusters "${CLUSTER_NAME}"
}

function start_cluster() {
  if (kind get clusters | grep "^${CLUSTER_NAME}$"); then
    if [[ "${REUSE_KIND_CLUSTER_IF_EXISTS}" = true ]]; then
      printf "KinD cluster exists and REUSE_KIND_CLUSTER_IF_EXISTS = true, so using existing KinD cluster.\n\n"
      kind export kubeconfig --name "${CLUSTER_NAME}"
      setup_cluster
      return
    else
      printf "Delete KinD cluster, so the test starts with a clean slate.\n\n"
      delete_cluster
    fi
  fi

  echo "Create test Kubernetes ${KUBERNETES_VERSION} cluster in KinD. This will allow us to test the CRDs, Helm chart and the Docker image."
  kind create cluster --name "${CLUSTER_NAME}" --image "kindest/node:${KUBERNETES_VERSION}" --config "${SCRIPT_DIR}/e2e-kind-config.yaml"

  setup_cluster
}

function setup_cluster() {
  # Load the docker images into the cluster
  add_image_to_kind_repo_if_local "${OPERATOR_IMAGE}" false
  add_image_to_kind_repo_if_local "${SOLR_IMAGE}" true
  for IMAGE in "${ADDITIONAL_IMAGES[@]}"; do
    add_image_to_kind_repo_if_local "${IMAGE}" true
  done

  printf "Installing Solr & Zookeeper CRDs\n"
  kubectl create -f "${REPO_DIR}/config/crd/bases/" 2>/dev/null || kubectl replace -f "${REPO_DIR}/config/crd/bases/"
  kubectl create -f "${REPO_DIR}/config/dependencies/" 2>/dev/null || kubectl replace -f "${REPO_DIR}/config/dependencies/"
  echo ""
}

case "$ACTION" in
  run-tests)
    run_tests
    ;;
  create-cluster)
    echo "Creating test Kubernetes ${KUBERNETES_VERSION} cluster in KinD. This will allow us to run end-to-end tests."
    start_cluster
    ;;
  destroy-cluster)
    echo "Deleting test KinD Kubernetes cluster."
    delete_cluster
    ;;
  kubeconfig)
    export_kubeconfig
    ;;
  *)
    show_help >&2
    echo "" >&2
    echo "Invalid action '${ACTION}' provided" >&2
    exit 1
    ;;
esac
