#!/usr/bin/env bash
# exit immediately when a command fails
set -e
# only exit with zero if all commands of the pipeline exit successfully
set -o pipefail
# error on unset variables
set -u

show_help() {
cat << EOF
Usage: ./hack/release/artifacts/bundle_source.sh [-h] [-v VERSION] -d ARTIFACTS_DIR

Setup the source release artifact.

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

echo "Bundling source release for the Solr Operator ${VERSION} at '${ARTIFACTS_DIR}/source/solr-operator-${VERSION}.tgz'"

TAR="${TAR:=tar}"
if ! ("${TAR}" --version | grep "GNU tar"); then
  TAR="gtar"
  if ! ("${TAR}" --version | grep "GNU tar"); then
    printf "\nMust have a GNU tar installed. If on OSX, then please download gnu-tar. It is available via brew.\nThe GNU version of Tar must be available at either 'tar' or 'gtar'.\n"
    exit 1
  fi
fi

# Setup directory
mkdir -p "${ARTIFACTS_DIR}"/source
rm -rf "${ARTIFACTS_DIR}"/source/*

# Package all of the code into a source release

COPYFILE_DISABLE=true "${TAR}" --exclude-vcs --exclude-vcs-ignores \
  --exclude .asf.yaml \
  --exclude .github \
  --transform "s,^,solr-operator-${VERSION}/," \
  -czf "${ARTIFACTS_DIR}/source/solr-operator-${VERSION}.tgz" .
