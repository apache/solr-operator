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
Usage: ./hack/release/artifacts/build_helm.sh [-h] [-v VERSION] [-a APACHE_ID] -d ARTIFACTS_DIR

Build the helm chart & repo.

    -h  Display this help and exit
    -v  Version of the Solr Operator (Optional, will default to project version)
    -d  Base directory of the staged artifacts.
    -a  Apache ID, to find the GPG key when signing the helm chart (Optional)
EOF
}

OPTIND=1
while getopts hv:d:a: opt; do
    case $opt in
        h)
            show_help
            exit 0
            ;;
        v)  VERSION=$OPTARG
            ;;
        a)  APACHE_ID=$OPTARG
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

HELM_RELEASE_DIR="${ARTIFACTS_DIR}/helm-charts"
echo "Packaging helm chart for version ${VERSION} at: ${HELM_RELEASE_DIR}"

# Setup directory
mkdir -p "${HELM_RELEASE_DIR}"
rm -rf "${HELM_RELEASE_DIR}"/*

# Package and Index the helm charts

SIGNING_INFO=()
CREATED_SECURE_RING=false
SECURE_RING_FILE=~/.gnupg/secring.gpg
if [[ -n "${APACHE_ID:-}" ]]; then
  # First generate the temporary secret key ring
  if [[ ! -f "${SECURE_RING_FILE}" ]]; then
    gpg --export-secret-keys >"${SECURE_RING_FILE}"
    CREATED_SECURE_RING=true
  fi

  SIGNING_INFO=(--sign --key "${APACHE_ID}@apache.org" --keyring "${SECURE_RING_FILE}")
  printf "\n\nPackaging and signing helm chart. Please provide the passphrase for your GPG key when prompted below.\nThe prompt may be subtle, but look at the cursor.\n"
fi

helm package -u helm/solr-operator --app-version "${VERSION}" --version "${VERSION#v}" -d "${HELM_RELEASE_DIR}" "${SIGNING_INFO[@]}"
helm package -u helm/solr --version "${VERSION#v}" -d "${HELM_RELEASE_DIR}" "${SIGNING_INFO[@]}"

if [[ "${CREATED_SECURE_RING}" = true ]]; then
  # Remove the temporary secret key ring
  rm "${SECURE_RING_FILE}"
fi

helm repo index "${HELM_RELEASE_DIR}"

{
  cat hack/headers/header.yaml.txt
  printf "\n\n"
  cat "${HELM_RELEASE_DIR}/index.yaml"
} > "${HELM_RELEASE_DIR}/index.yaml.tmp" && mv "${HELM_RELEASE_DIR}/index.yaml.tmp" "${HELM_RELEASE_DIR}/index.yaml"

