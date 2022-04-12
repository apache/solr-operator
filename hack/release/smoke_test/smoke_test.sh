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
Usage: ./hack/release/smoke_test/smoke_test.sh [-h] [-i IMAGE] [-s GIT_SHA] [-k KUBERNETES_VERSION] [-t SOLR_VERSION] [-g GPG_KEY] -v VERSION -l LOCATION

Smoke test the Solr Operator release artifacts.

    -h  Display this help and exit
    -v  Version of the Solr Operator
    -l  Base location of the staged artifacts. Can be a URL or relative or absolute file path.
    -i  Solr Operator Docker image to use  (Optional, defaults to apache/solr-operator:<version>)
    -s  GitSHA of the last commit for this version of Solr (Optional, check will not happen if not provided)
    -g  GPG Key (fingerprint) used to sign the artifacts (Optional, will not check signatures if not provided)
    -k  Kubernetes Version to test with (Optional, defaults to a compatible version)
    -t  Solr Image (or version/tag of the official Solr image) to test with (Optional, defaults to a compatible version)
EOF
}

OPTIND=1
while getopts hv:i:l:s:g:k:t: opt; do
    case $opt in
        h)
            show_help
            exit 0
            ;;
        v)  VERSION=$OPTARG
            ;;
        s)  GIT_SHA=$OPTARG
            ;;
        i)  IMAGE=$OPTARG
            ;;
        l)  LOCATION=$OPTARG
            ;;
        g)  GPG_KEY=$OPTARG
            ;;
        k)  KUBERNETES_VERSION=$OPTARG
            ;;
        t)  SOLR_IMAGE=$OPTARG
            ;;
        *)
            show_help >&2
            exit 1
            ;;
    esac
done
shift "$((OPTIND-1))"   # Discard the options and sentinel --

if [[ -z "${VERSION:-}" ]]; then
  echo "Specify a project version through -v, or through the VERSION env var" >&2 && exit 1
fi
if [[ -z "${IMAGE:-}" ]]; then
  IMAGE="apache/solr-operator:${VERSION}"
fi
if [[ -z "${LOCATION:-}" ]]; then
  echo "Specify an base artifact location -l, or through the LOCATION env var" >&2 && exit 1
fi

# Pass through the following options to child scripts, if they were given
if [[ -n "${KUBERNETES_VERSION:-}" ]]; then
  export KUBERNETES_VERSION="${KUBERNETES_VERSION}"
fi
if [[ -n "${SOLR_IMAGE:-}" ]]; then
  export SOLR_IMAGE="${SOLR_IMAGE}"
fi
if [[ -n "${GPG_KEY:-}" ]]; then
  export GPG_KEY="${GPG_KEY}"
fi

PULL_PASS_THROUGH=""
# If LOCATION is a URL, we want to pull the Docker image
if (echo "${LOCATION}" | grep "http"); then
  PULL_PASS_THROUGH="-p"
fi

GIT_SHA_PASS_THROUGH=()
if [[ -n "${GIT_SHA:-}" ]]; then
  GIT_SHA_PASS_THROUGH=(-s "${GIT_SHA}")
fi

# Add GOBIN to PATH
if [[ -z "${GOBIN:-}" ]]; then
  export GOBIN="$(cd "${GOPATH:-~/go}/bin" && pwd)"
fi
export PATH="${PATH}:${GOBIN}"

./hack/release/smoke_test/verify_all.sh -v "${VERSION}" -l "${LOCATION}"
./hack/release/smoke_test/verify_docker.sh -v "${VERSION}" -i "${IMAGE}" "${GIT_SHA_PASS_THROUGH[@]}" "${PULL_PASS_THROUGH}"
./hack/release/smoke_test/test_source.sh -v "${VERSION}" -l "${LOCATION}"
./hack/release/smoke_test/test_cluster.sh -v "${VERSION}" -i "${IMAGE}" -l "${LOCATION}"

printf "\n\n********************\nSuccessfully smoke tested the Solr Operator %s!\n" "${VERSION}"
