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
Usage: ./hack/release/smoke_test/verify_docker.sh [-h] [-p] [-i IMAGE] [-s GIT_SHA] -v VERSION

Verify the Solr Operator Docker image.

    -h  Display this help and exit
    -p  Pull Docker image before verifying (Optional, defaults to false)
    -v  Version of the Solr Operator
    -s  GitSHA of the last commit for this version of Solr (Optional, check will not happen if not provided)
    -i  Name of the docker image to verify (Optional, defaults to apache/solr-operator:<version>)
EOF
}

OPTIND=1
while getopts hpv:i:s: opt; do
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
        p)  PULL_FIRST=true
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

# Pull the image, if requested
if [[ ${PULL_FIRST:-} ]]; then
  docker pull "${IMAGE}"
fi

echo "Verify the Docker image ${IMAGE}"

# Check for the LICENSE and NOTICE info in the image
docker run --rm --entrypoint='sh' "${IMAGE}" -c "ls /etc/licenses/LICENSE ; ls /etc/licenses/NOTICE; ls /etc/licenses/dependencies/*"

# Check for Version and other information
SOLR_OP_LOGS=$(docker run --rm --entrypoint='sh' "${IMAGE}" -c "/solr-operator 2>&1 || true")
echo "${SOLR_OP_LOGS}" | grep "solr-operator Version: ${VERSION}" \
  || {
     printf "\n\n" >&2
     echo "Could not find correct Version in Operator startup logs: ${VERSION}" >&2;
     printf "\n\n" >&2
     echo "${SOLR_OP_LOGS}" >&2;
     exit 1
    }
if [[ -n "${GIT_SHA:-}" ]]; then
  echo "${SOLR_OP_LOGS}" | grep "solr-operator Git SHA: ${GIT_SHA}" \
    || {
     printf "\n\n" >&2
     echo "Could not find correct Git SHA in Operator startup logs: ${GIT_SHA}" >&2;
     printf "\n\n" >&2
     echo "${SOLR_OP_LOGS}" >&2;
     exit 1
    }
fi

printf "\n********************\nDocker verification successful!\n\n"
