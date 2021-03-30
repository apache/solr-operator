#!/usr/bin/env bash
# exit immediately when a command fails
set -e
# only exit with zero if all commands of the pipeline exit successfully
set -o pipefail
# error on unset variables
set -u


###
# Remove information in the project specific to a previous version.
# Use:
#   ./hack/release/version/remove_version_specific_info.sh
# Requires:
#  - yq
###

echo "Removing information specific to the previous release"

# Reset ArtifactHub changelog in Chart.yaml
yq -i eval '.annotations."artifacthub.io/changes" |= "- Change 1
- Change 2
"' helm/solr-operator/Chart.yaml
