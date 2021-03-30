#!/usr/bin/env bash
# exit immediately when a command fails
set -e
# only exit with zero if all commands of the pipeline exit successfully
set -o pipefail
# error on unset variables
set -u

###
# Test the release source bundle
# Download can either be a URL of the base release directory, or a relative or absolute filepath.
# Use:
#   VERSION="v0.3.5" DOWNLOAD="https://dist.apache.org/repos/dist/dev/solr/solr-operator/..." ./hack/release/smoke_test/test_source.sh
#   ./hack/release/smoke_test/test_source.sh "v0.2.8" "https://dist.apache.org/repos/dist/dev/solr/solr-operator/...."
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

TMP_DIR=$(mktemp -d --tmpdir "solr-operator-smoke-test-source-XXXXXXXX")

# If DOWNLOAD is not a URL, then get the absolute path
if ! (echo "${DOWNLOAD}" | grep -E "http"); then
  DOWNLOAD=$(cd "${DOWNLOAD}"; pwd)
fi

echo "Download source artifact, verify and run 'make check'"
# Do all logic in temporary directory
(
  cd "${TMP_DIR}"

  if (echo "${DOWNLOAD}" | grep -E "http"); then
    # Download source & asc
    wget "${DOWNLOAD}/source/solr-operator-${VERSION}.tgz"
  else
    cp "${DOWNLOAD}/source/solr-operator-${VERSION}.tgz" .
  fi

  # Unpack the source code
  tar -xzf "solr-operator-${VERSION}.tgz"
  cd "solr-operator-${VERSION}"

  # Install the dependencies
  make install-dependencies

  # Run the checks
  make check

  # Check the version
  FOUND_VERSION=$(make version)
  if [[ "$FOUND_VERSION" != "${VERSION}" ]]; then
    error "Version in source release should be ${VERSION}, but found ${FOUND_VERSION}"
    exit 1
  fi
)

# Delete temporary source download directory
rm -rf "${TMP_DIR}"

echo "Source verification successful!"