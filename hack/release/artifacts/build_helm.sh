#!/usr/bin/env bash
# exit immediately when a command fails
set -e
# only exit with zero if all commands of the pipeline exit successfully
set -o pipefail
# error on unset variables
set -u

show_help() {
cat << EOF
Usage: ./hack/release/artifacts/build_helm.sh [-h] [-v VERSION] -d ARTIFACTS_DIR

Build the helm chart & repo.

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

echo "Packaging helm chart for version ${VERSION} at release-artifacts/helm"

# Setup directory
mkdir -p "${ARTIFACTS_DIR}"/helm
rm -rf "${ARTIFACTS_DIR}"/helm/*

# Package and Index the helm charts, create release artifacts to upload in GithubRelease

helm dependency build helm/solr-operator

helm package -u helm/* --app-version "${VERSION}" --version "${VERSION#v}" -d "${ARTIFACTS_DIR}/helm"

helm repo index "${ARTIFACTS_DIR}/helm"
