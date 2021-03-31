#!/usr/bin/env bash
# exit immediately when a command fails
set -e
# only exit with zero if all commands of the pipeline exit successfully
set -o pipefail
# error on unset variables
set -u

show_help() {
cat << EOF
Usage: ./hack/release/version/update_version.sh [-h] -v VERSION

Change the Version of the project.

    -h  Display this help and exit
    -s  New version suffix for the project. (Optional)
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
        *)
            show_help >&2
            exit 1
            ;;
    esac
done
shift "$((OPTIND-1))"   # Discard the options and sentinel --

if [[ -z "${VERSION:-}" ]]; then
  error "Specify a new project version through -v, or through the VERSION env var"; exit 1
fi

echo "Updating the latest version throughout the repo to: ${VERSION}"

# Version file
awk -i inplace '$1 == "Version"{$4 = "\"'"${VERSION}"'\""} 1' version/version.go && \
  go fmt version/version.go
