#!/usr/bin/env bash
# exit immediately when a command fails
set -e
# only exit with zero if all commands of the pipeline exit successfully
set -o pipefail
# error on unset variables
set -u

echo "Setting up Release ${VERSION}, making last commit."

# Package and Index the helm charts, create release artifacts to upload in GithubRelease
cp config/crd/bases/* release-artifacts/.

git add helm config docs

git commit -asm "Cutting release version ${VERSION} of the Solr Operator"
