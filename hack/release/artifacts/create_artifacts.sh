#!/usr/bin/env bash
# exit immediately when a command fails
set -e
# only exit with zero if all commands of the pipeline exit successfully
set -o pipefail
# error on unset variables
set -u

###
# Setup the release of the CRDs.
# Use:
#   ./hack/release/artifact/create_artifacts.sh
#         (RELEASE_ARTIFACTS_DIR will default to "release-artifacts")
#   RELEASE_ARTIFACTS_DIR=../release-artifacts ./hack/release/artifact/create_artifacts.sh
#   ./hack/release/artifact/create_artifacts.sh ../release-artifacts
###
if [[ -z "${RELEASE_ARTIFACTS_DIR:-}" ]]; then
  if [[ -z "${1:-}" ]]; then
    export RELEASE_ARTIFACTS_DIR="release-artifacts"
  else
    export RELEASE_ARTIFACTS_DIR="$1"
  fi
fi

VERSION=$(make version)

echo "Setting up Solr Operator ${VERSION} release artifacts at '${RELEASE_ARTIFACTS_DIR}'"

RELEASE_ARTIFACTS_DIR="${RELEASE_ARTIFACTS_DIR}" ./hack/release/artifacts/bundle_source.sh
RELEASE_ARTIFACTS_DIR="${RELEASE_ARTIFACTS_DIR}" ./hack/release/artifacts/create_crds.sh
RELEASE_ARTIFACTS_DIR="${RELEASE_ARTIFACTS_DIR}" ./hack/release/artifacts/build_helm.sh