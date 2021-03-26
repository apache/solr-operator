#!/usr/bin/env bash
# exit immediately when a command fails
set -e
# only exit with zero if all commands of the pipeline exit successfully
set -o pipefail
# error on unset variables
set -u


###
# Change the Version Suffix of the project.
# Use:
#   ./hack/release/version/change_suffix.sh
#           (This will remove the suffix if one exists, or change it to "prerelease" if empty)
#   VERSION_SUFFIX=new-suffix ./hack/release/version/change_suffix.sh
#   ./hack/release/version/change_suffix.sh new-suffix
###


if [[ -z "${VERSION_SUFFIX:-}" ]]; then
  if [[ -z "${1:-}" ]]; then
    EXISTING_VERSION_SUFFIX="$(cat version/version.go | grep -E 'VersionSuffix([[:space:]]+)string' | grep -o '["''].*["'']' | xargs)"
    if [[ -z "${EXISTING_VERSION_SUFFIX}" ]]; then
      export VERSION_SUFFIX="prerelease"
    else
      export VERSION_SUFFIX=""
    fi
  else
    export VERSION_SUFFIX="$1"
  fi
fi

echo "Updating the version suffix for the project to: ${VERSION_SUFFIX}"

# Version file
awk -i inplace '$1 == "VersionSuffix"{$4 = "\"'"${VERSION_SUFFIX}"'\""} 1' version/version.go && \
  go fmt version/version.go
