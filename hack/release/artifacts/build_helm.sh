#!/usr/bin/env bash
# exit immediately when a command fails
set -e
# only exit with zero if all commands of the pipeline exit successfully
set -o pipefail
# error on unset variables
set -u

###
# Build Helm Chart.
# Use:
#   ./hack/release/artifact/build_helm.sh
#         (RELEASE_ARTIFACTS_DIR will default to "release-artifacts")
#   RELEASE_ARTIFACTS_DIR=../release-artifacts ./hack/release/artifact/build_helm.sh
#   ./hack/release/artifact/build_helm.sh ../release-artifacts
###
if [[ -z "${RELEASE_ARTIFACTS_DIR:-}" ]]; then
  if [[ -z "${1:-}" ]]; then
    export RELEASE_ARTIFACTS_DIR="release-artifacts"
  else
    export RELEASE_ARTIFACTS_DIR="$1"
  fi
fi

VERSION=$(make version)

echo "Packaging helm chart for version ${VERSION} at release-artifacts/helm"

# Setup directory
mkdir -p "${RELEASE_ARTIFACTS_DIR}"/helm
rm -rf "${RELEASE_ARTIFACTS_DIR}"/helm/*

# Package and Index the helm charts, create release artifacts to upload in GithubRelease

helm dependency build helm/solr-operator

helm package -u helm/* --app-version "${VERSION}" --version "${VERSION#v}" -d "${RELEASE_ARTIFACTS_DIR}/helm"

helm repo index "${RELEASE_ARTIFACTS_DIR}/helm"
