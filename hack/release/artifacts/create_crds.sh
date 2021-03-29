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
#   ./hack/release/artifact/create_crds.sh
#         (RELEASE_ARTIFACTS_DIR will default to "release-artifacts")
#   RELEASE_ARTIFACTS_DIR=../release-artifacts ./hack/release/artifact/create_crds.sh
#   ./hack/release/artifact/create_crds.sh ../release-artifacts
###
if [[ -z "${RELEASE_ARTIFACTS_DIR:-}" ]]; then
  if [[ -z "${1:-}" ]]; then
    export RELEASE_ARTIFACTS_DIR="release-artifacts"
  else
    export RELEASE_ARTIFACTS_DIR="$1"
  fi
fi

VERSION=$(make version)

echo "Setting up Solr Operator ${VERSION} CRDs at ${RELEASE_ARTIFACTS_DIR}/crds."

# Setup directory
mkdir -p "${RELEASE_ARTIFACTS_DIR}"/crds
rm -rf "${RELEASE_ARTIFACTS_DIR}"/crds/*

# Create Release CRD files
{
  cat hack/headers/header.yaml.txt
  printf "\n"
} > "${RELEASE_ARTIFACTS_DIR}/crds/all.yaml"
for filename in config/crd/bases/*.yaml; do
    output_file=${filename#"config/crd/bases/solr.apache.org_"}
    # Create individual file with Apache Header
    {
      cat hack/headers/header.yaml.txt;
      printf "\n"
      cat "${filename}";
    } > "${RELEASE_ARTIFACTS_DIR}/crds/${output_file}"

    # Add to aggregate file
    cat "${filename}" >> "${RELEASE_ARTIFACTS_DIR}/crds/all.yaml"
done

# Fetch the correct dependency Zookeeper CRD, package with other CRDS
{
  cat hack/headers/zookeeper-operator-header.yaml.txt;
  printf "\n\n---\n"
  curl -sL "https://raw.githubusercontent.com/pravega/zookeeper-operator/v0.2.9/deploy/crds/zookeeper.pravega.io_zookeeperclusters_crd.yaml"
} > "${RELEASE_ARTIFACTS_DIR}/crds/zookeeperclusters.yaml"

# Package all Solr and Dependency CRDs
{
  cat "${RELEASE_ARTIFACTS_DIR}/crds/all.yaml"
  printf "\n"
  cat "${RELEASE_ARTIFACTS_DIR}/crds/zookeeperclusters.yaml"
} > "${RELEASE_ARTIFACTS_DIR}/crds/all-with-dependencies.yaml"
