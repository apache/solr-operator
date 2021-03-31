#!/usr/bin/env bash
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

while getopts hvf: opt; do
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
  VERSION=$(make version)
fi
if [[ -z "${ARTIFACTS_DIR:-}" ]]; then
  error "Specify an base artifact directory -d, or through the ARTIFACTS_DIR env var"; exit 1
fi

echo "Setting up Solr Operator ${VERSION} CRDs at ${RELEASE_ARTIFACTS_DIR}/crds."

# Setup directory
mkdir -p "${ARTIFACTS_DIR}"/crds
rm -rf "${ARTIFACTS_DIR}"/crds/*

# Create Release CRD files
{
  cat hack/headers/header.yaml.txt
  printf "\n"
} > "${ARTIFACTS_DIR}/crds/all.yaml"
for filename in config/crd/bases/*.yaml; do
    output_file=${filename#"config/crd/bases/solr.apache.org_"}
    # Create individual file with Apache Header
    {
      cat hack/headers/header.yaml.txt;
      printf "\n"
      cat "${filename}";
    } > "${ARTIFACTS_DIR}/crds/${output_file}"

    # Add to aggregate file
    cat "${filename}" >> "${ARTIFACTS_DIR}/crds/all.yaml"
done

# Fetch the correct dependency Zookeeper CRD, package with other CRDS
{
  cat hack/headers/zookeeper-operator-header.yaml.txt;
  printf "\n\n---\n"
  curl -sL "https://raw.githubusercontent.com/pravega/zookeeper-operator/v0.2.9/deploy/crds/zookeeper.pravega.io_zookeeperclusters_crd.yaml"
} > "${ARTIFACTS_DIR}/crds/zookeeperclusters.yaml"

# Package all Solr and Dependency CRDs
{
  cat "${ARTIFACTS_DIR}/crds/all.yaml"
  printf "\n"
  cat "${ARTIFACTS_DIR}/crds/zookeeperclusters.yaml"
} > "${ARTIFACTS_DIR}/crds/all-with-dependencies.yaml"
