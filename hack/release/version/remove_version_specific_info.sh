#!/usr/bin/env bash
# exit immediately when a command fails
set -e
# only exit with zero if all commands of the pipeline exit successfully
set -o pipefail
# error on unset variables
set -u

show_help() {
cat << EOF
Usage: ./hack/release/version/remove_version_specific_info.sh [-h]

Remove information in the project specific to a previous version.
Requires:
 * yq

    -h  Display this help and exit
EOF
}

OPTIND=1

while getopts hvf: opt; do
    case $opt in
        h)
            show_help
            exit 0
            ;;
        *)
            show_help >&2
            exit 1
            ;;
    esac
done
shift "$((OPTIND-1))"   # Discard the options and sentinel --

echo "Removing information specific to the previous release"

# Reset ArtifactHub changelog in Chart.yaml
yq -i eval '.annotations."artifacthub.io/changes" |= "- Change 1
- Change 2
"' helm/solr-operator/Chart.yaml
