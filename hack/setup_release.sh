#!/usr/bin/env bash
# exit immediately when a command fails
set -e
# only exit with zero if all commands of the pipeline exit successfully
set -o pipefail
# error on unset variables
set -u

echo "Setting up Release ${VERSION}"

# Update default solr-operator version and the helm chart versions.
gawk -i inplace '$1 == "repository:" { tag = ($2 == "bloomberg/solr-operator") }
tag && $1 == "tag:"{$1 = "  " $1; $2 = "'"${VERSION}"'"} 1' helm/solr-operator/values.yaml

gawk -i inplace '$1 == "version:"{$1 = $1; $2 = "'"${VERSION#v}"'"} 1' helm/solr-operator/Chart.yaml
gawk -i inplace '$1 == "appVersion:"{$1 = $1; $2 = "'"${VERSION}"'"} 1' helm/solr-operator/Chart.yaml


# Package and Index the helm charts, create release artifacts to upload in GithubRelease
mkdir -p release-artifacts

rm -rf release-artifacts/*

helm package helm/* --app-version "${VERSION}" --version "${VERSION#v}" -d release-artifacts/

helm repo index release-artifacts/ --url https://github.com/bloomberg/solr-operator/releases/download/${VERSION}/ --merge docs/charts/index.yaml

mv release-artifacts/index.yaml docs/charts/index.yaml

cp config/crd/bases/* release-artifacts/.