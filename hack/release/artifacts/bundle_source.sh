#!/usr/bin/env bash
# exit immediately when a command fails
set -e
# only exit with zero if all commands of the pipeline exit successfully
set -o pipefail
# error on unset variables
set -u

###
# Setup source release artifact.
# Use:
#   ./hack/release/artifact/bundle_source.sh
#         (RELEASE_ARTIFACTS_DIR will default to "release-artifacts")
#   RELEASE_ARTIFACTS_DIR=../release-artifacts ./hack/release/artifact/bundle_source.sh
#   ./hack/release/artifact/bundle_source.sh ../release-artifacts
###
if [[ -z "${RELEASE_ARTIFACTS_DIR:-}" ]]; then
  if [[ -z "${1:-}" ]]; then
    export RELEASE_ARTIFACTS_DIR="release-artifacts"
  else
    export RELEASE_ARTIFACTS_DIR="$1"
  fi
fi

VERSION=$(make version)

echo "Bundling source release for the Solr Operator ${VERSION} at '${RELEASE_ARTIFACTS_DIR}/source/solr-operator-${VERSION}.tgz'"

TAR="${TAR:=tar}"
if ! ("${TAR}" --version | grep "GNU tar"); then
  TAR="gtar"
  if ! ("${TAR}" --version | grep "GNU tar"); then
    printf "\nMust have a GNU tar installed. If on OSX, then please download gnu-tar. It is available via brew.\nThe GNU version of Tar must be available at either 'tar' or 'gtar'.\n"
    exit 1
  fi
fi

# Setup directory
mkdir -p "${RELEASE_ARTIFACTS_DIR}"/source
rm -rf "${RELEASE_ARTIFACTS_DIR}"/source/*

# Package all of the code into a source release

COPYFILE_DISABLE=true "${TAR}" --exclude-vcs --exclude-vcs-ignores \
  --exclude .asf.yaml \
  --exclude .github \
  --transform "s,^,solr-operator-${VERSION}/," \
  -czf "${RELEASE_ARTIFACTS_DIR}/source/solr-operator-${VERSION}.tgz" .
