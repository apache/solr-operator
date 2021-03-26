#!/usr/bin/env bash
# exit immediately when a command fails
set -e
# only exit with zero if all commands of the pipeline exit successfully
set -o pipefail
# error on unset variables
set -u

###
# Change the Version of the project
# Use:
#   VERSION=v1.2.3 ./hack/release/version/update_version.sh
#   ./hack/release/version/update_version.sh v1.2.3
###

if [[ -z "${VERSION:-}" ]]; then
  export VERSION="$1"
fi

echo "Updating the latest version throughout the repo to: ${VERSION}"

# Version file
awk -i inplace '$1 == "Version"{$4 = "\"'"${VERSION}"'\""} 1' version/version.go && \
  go fmt version/version.go
