#!/usr/bin/env bash
# exit immediately when a command fails
set -e
# only exit with zero if all commands of the pipeline exit successfully
set -o pipefail
# error on unset variables
set -u

echo "Packaging helm chart for version ${VERSION}"

# Package and Index the helm charts, create release artifacts to upload in GithubRelease
mkdir -p release-artifacts

rm -rf release-artifacts/*

helm package -u helm/* --app-version "${VERSION}" --version "${VERSION#v}" -d release-artifacts/

helm repo index release-artifacts/ --url https://github.com/apache/lucene-solr-operator/releases/download/${VERSION}/ --merge docs/charts/index.yaml

mv release-artifacts/index.yaml docs/charts/index.yaml
