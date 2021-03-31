#!/usr/bin/env bash
# exit immediately when a command fails
set -e
# only exit with zero if all commands of the pipeline exit successfully
set -o pipefail
# error on unset variables
set -u

show_help() {
cat << EOF
Usage: ./hack/release/smoke_test/smoke_test.sh [-h] -v VERSION -t TAG -l LOCATION

Smoke test the Solr Operator release artifacts.

    -h  Display this help and exit
    -v  Version of the Solr Operator
    -t  Tag of the Solr Operator docker image to use
    -l  Base location of the staged artifacts. Can be a URL or relative or absolute file path.
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
        t)  TAG=$OPTARG
            ;;
        l)  LOCATION=$OPTARG
            ;;
        *)
            show_help >&2
            exit 1
            ;;
    esac
done
shift "$((OPTIND-1))"   # Discard the options and sentinel --

if [[ -z "${VERSION:-}" ]]; then
  error "Specify a project version through -v, or through the VERSION env var"; exit 1
fi
if [[ -z "${TAG:-}" ]]; then
  error "Specify a docker image tag through -v, or through the TAG env var"; exit 1
fi
if [[ -z "${LOCATION:-}" ]]; then
  error "Specify an base artifact location -l, or through the LOCATION env var"; exit 1
fi

./hack/release/smoke_test/verify_all.sh -v "${VERSION}" -t "${TAG}" -l "${LOCATION}"
./hack/release/smoke_test/test_source.sh -v "${VERSION}" -l "${LOCATION}"
./hack/release/smoke_test/test_cluster.sh -v "${VERSION}" -t "${TAG}" -l "${LOCATION}"

printf "\n\nSuccessfully smoke tested the Solr Operator %s!\n" "${VERSION}"