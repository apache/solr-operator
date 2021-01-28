#!/usr/bin/env bash
# exit immediately when a command fails
set -e
# only exit with zero if all commands of the pipeline exit successfully
set -o pipefail
# error on unset variables
set -u

echo "Updating the latest version throughout the repo to: ${VERSION}"

# Update default solr-operator version and the helm chart versions.
gawk -i inplace '$1 == "repository:" { tag = ($2 == "apache/solr-operator") }
tag && $1 == "tag:"{$1 = "  " $1; $2 = "'"${VERSION}"'"} 1' helm/solr-operator/values.yaml

gawk -i inplace '$1 == "version:"{$1 = $1; $2 = "'"${VERSION#v}"'"} 1' helm/solr-operator/Chart.yaml
gawk -i inplace '$1 == "appVersion:"{$1 = $1; $2 = "'"${VERSION}"'"} 1' helm/solr-operator/Chart.yaml
sed -i.bak -E 's/^\| image.tag \| string \| `".*"` \|/\| image.tag \| string \| `"'${VERSION}'"` \|/g' helm/solr-operator/README.md && rm helm/solr-operator/README.md.bak