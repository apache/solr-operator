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

show_help() {
cat << EOF
Usage: ./tests/util/setup_cluster.sh (create|destroy|kubeconfig) [-h] [-k KUBERNETES_VERSION] [-s SOLR_IMAGE] [-a ADDITIONAL_IMAGES (bash array)] -i OPERATOR_IMAGE

Manage a KinD cluster for Solr Operator e2e testing.

Available actions are: create, destroy, kubeconfig

    -h  Display this help and exit
    -i  Solr Operator docker image to use (Optional, defaults to apache/solr-operator:<version>)
    -k  Kubernetes Version to test with (full tag, e.g. v1.21.2) (Optional, defaults to a compatible version)
    -s  Full solr image, or image tag (for the official Solr image), to test with (e.g. apache/solr-nightly:9.0.0, 8.11). (Optional, defaults to a compatible version)
    -a  Load additional local images into the test Kubernetes cluster. Provide option multiple times for multiple images. (Optional)
EOF
}

ADDITIONAL_IMAGES=()
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
echo $1
ACTION="${1:-}"
if [[ -z "${ACTION:-}" ]]; then
  echo "You must specify a cluster action. Either 'create', 'destroy' or 'kubeconfig'." >&2 && exit 1
fi

if [[ -z "${OPERATOR_IMAGE:-}" ]]; then
  echo "Specify a Docker image for the Solr Operator through -i, or through the OPERATOR_IMAGE env var" >&2 && exit 1
fi
if [[ -z "${KUBERNETES_VERSION:-}" ]]; then
  KUBERNETES_VERSION="v1.21.14"
fi
if [[ -z "${SOLR_IMAGE:-}" ]]; then
  SOLR_IMAGE="${SOLR_VERSION:-9.0}"
fi
if [[ "${SOLR_IMAGE}" != *":"* ]]; then
  SOLR_IMAGE="solr:${SOLR_IMAGE}"
fi

export CLUSTER_NAME="solr-op-${OPERATOR_IMAGE##*:}-e2e-tests-solr-${SOLR_IMAGE##*:}"
export KUBE_CONTEXT="kind-${CLUSTER_NAME}"
export KUBERNETES_VERSION="${KUBERNETES_VERSION}"
export OPERATOR_IMAGE="${OPERATOR_IMAGE}"
export SOLR_IMAGE="${SOLR_IMAGE}"
export ADDITIONAL_IMAGES=("${ADDITIONAL_IMAGES[@]}")


function add_image_to_kind_repo_if_local() {
  IMAGE="$1"
  if (docker image inspect "${IMAGE}" &>/dev/null); then
    kind load docker-image --name "${CLUSTER_NAME}" "${IMAGE}"
    printf "\nUsing local version of image \"%s\".\nIf you want to use an updated version of this image, run \"docker pull %s\" before running the smoke test again.\n\n" "${IMAGE}" "${IMAGE}"
  else
    printf "\nUsing the remote image \"%s\", since it was not found in the local Docker image list.\n\n" "${IMAGE}"
  fi
}

function export_kubeconfig() {
  kind export kubeconfig --name "${CLUSTER_NAME}"
}

function delete_cluster() {
  kind delete clusters "${CLUSTER_NAME}"
}

function start_cluster() {
  if (kind get clusters | grep "${CLUSTER_NAME}"); then
    printf "Delete cluster, so the test starts with a clean slate.\n\n"
    delete_cluster
  fi

  echo "Create test Kubernetes ${KUBERNETES_VERSION} cluster in Kind. This will allow us to test the CRDs, Helm chart and the Docker image."
  kind create cluster --name "${CLUSTER_NAME}" --image "kindest/node:${KUBERNETES_VERSION}"

  # Load the docker images into the cluster
  add_image_to_kind_repo_if_local "${OPERATOR_IMAGE}"
  add_image_to_kind_repo_if_local "${SOLR_IMAGE}"
  for IMAGE in "${ADDITIONAL_IMAGES[@]}"; do
    add_image_to_kind_repo_if_local "${IMAGE}"
  done
}

case "$ACTION" in
  create)
    echo "Creating test Kubernetes ${KUBERNETES_VERSION} cluster in Kind. This will allow us to run end-to-end tests."
    start_cluster
    kubectl create -f "config/crd/bases/" || kubectl replace -f "config/crd/bases/"
    kubectl create -f "config/dependencies/" || kubectl replace -f "config/dependencies/"
    ;;
  destroy)
    echo "Deleting test Kind Kubernetes cluster."
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
