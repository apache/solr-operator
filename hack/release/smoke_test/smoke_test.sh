#!/usr/bin/env bash
# exit immediately when a command fails
set -e
# only exit with zero if all commands of the pipeline exit successfully
set -o pipefail
# error on unset variables
set -u

###
# Smoke Test the release artifacts
# Download can either be a URL of the base release directory, or a relative or absolute filepath.
# Use:
#   VERSION="v0.3.5" DOWNLOAD="https://dist.apache.org/repos/dist/dev/solr/solr-operator/..." ./hack/release/smoke_test/smoke_test.sh
#   ./hack/release/smoke_test/smoke_test.sh "v0.2.8" "release-artifacts"
###
if [[ -z "${VERSION:-}" ]]; then
  if [[ -z "${1:-}" ]]; then
    error "Specify a version through the first argument, or through the VERSION env var"
    exit 1
  else
    export VERSION="$1"
  fi
fi
if [[ -z "${DOWNLOAD:-}" ]]; then
  if [[ -z "${2:-}" ]]; then
    error "Specify a download location through the second argument, or through the DOWNLOAD env var"
    exit 1
  else
    export DOWNLOAD="$2"
  fi
fi

VERSION="${VERSION}" DOWNLOAD="${DOWNLOAD}" ./hack/release/smoke_test/verify_all.sh
VERSION="${VERSION}" DOWNLOAD="${DOWNLOAD}" ./hack/release/smoke_test/test_source.sh
#VERSION="${VERSION}" DOWNLOAD="${DOWNLOAD}" ./hack/release/smoke_test/test_cluster.sh

printf "\n\nSuccessfully smoke tested the Solr Operator %s!" "${VERSION}\n"