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
Usage: ./hack/release/artifacts/create_crds.sh [-h] [-v VERSION] -d ARTIFACTS_DIR

Setup the release of the CRDs.

    -h  Display this help and exit
    -v  Version of the Solr Operator (Optional, will default to project version)
    -d  Base directory of the staged artifacts.
EOF
}

OPTIND=1
while getopts hv:d: opt; do
    case $opt in
        h)
            show_help
            exit 0
            ;;
        v)  VERSION=$OPTARG
            ;;
        d)  ARTIFACTS_DIR=$OPTARG
            ;;
        *)
            show_help >&2
            exit 1
            ;;
    esac
done
shift "$((OPTIND-1))"   # Discard the options and sentinel --

if [[ -z "${VERSION:-}" ]]; then
  VERSION=$(make -s version)
fi
if [[ -z "${ARTIFACTS_DIR:-}" ]]; then
  echo "Specify an base artifact directory -d, or through the ARTIFACTS_DIR env var" >&2 && exit 1
fi

echo "Setting up Solr Operator ${VERSION} CRDs at ${ARTIFACTS_DIR}/crds."

# Setup directory
mkdir -p "${ARTIFACTS_DIR}"/crds
rm -rf "${ARTIFACTS_DIR}"/crds/*

header_end="+$((1 + $(grep -c '^' hack/headers/header.yaml.txt)))"

# Create Release CRD files
{
  cat hack/headers/header.yaml.txt
  printf "\n"
} > "${ARTIFACTS_DIR}/crds/all.yaml"
for filename in config/crd/bases/*.yaml; do
    output_file=${filename#"config/crd/bases/solr.apache.org_"}
    # Copy individual yaml with condensed name
    cp "${filename}" "${ARTIFACTS_DIR}/crds/${output_file}"

    # Add to aggregate file, without header
    {
      tail -n "${header_end}" "${filename}";
      printf "\n"
    } >> "${ARTIFACTS_DIR}/crds/all.yaml"
done

# Fetch the correct dependency Zookeeper CRD, package with other CRDS
./hack/zk-operator/update-crd.sh
cp "config/dependencies/zookeeper_cluster_crd.yaml" "${ARTIFACTS_DIR}/crds/zookeeperclusters.yaml"

# Package all Solr and Dependency CRDs
{
  cat "${ARTIFACTS_DIR}/crds/all.yaml"
  printf "\n"
  cat "${ARTIFACTS_DIR}/crds/zookeeperclusters.yaml"
} > "${ARTIFACTS_DIR}/crds/all-with-dependencies.yaml"
