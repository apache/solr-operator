#!/usr/bin/env bash
# exit immediately when a command fails
set -e
# only exit with zero if all commands of the pipeline exit successfully
set -o pipefail
# error on unset variables
set -u

show_help() {
cat << EOF
Usage: ./hack/release/version/change_suffix.sh [-h] [-s SUFFIX]

Change the Version Suffix of the project.
If a suffix is not provided, the project suffix will flip between "" and "prerelease", depending on the current value.

    -h  Display this help and exit
    -s  New version suffix for the project. (Optional)
EOF
}

OPTIND=1
while getopts hs: opt; do
    case $opt in
        h)
            show_help
            exit 0
            ;;
        s)  VERSION_SUFFIX=$OPTARG
            ;;
        *)
            show_help >&2
            exit 1
            ;;
    esac
done
shift "$((OPTIND-1))"   # Discard the options and sentinel --

if [[ -z "${VERSION_SUFFIX:-}" ]]; then
  EXISTING_VERSION_SUFFIX="$(cat version/version.go | grep -E 'VersionSuffix([[:space:]]+)string' | grep -o '["''].*["'']' | xargs)"
  if [[ -z "${EXISTING_VERSION_SUFFIX}" ]]; then
    export VERSION_SUFFIX="prerelease"
  else
    export VERSION_SUFFIX=""
  fi
fi

echo "Updating the version suffix for the project to: ${VERSION_SUFFIX}"

# Version file
awk -i inplace '$1 == "VersionSuffix"{$4 = "\"'"${VERSION_SUFFIX}"'\""} 1' version/version.go && \
  go fmt version/version.go
