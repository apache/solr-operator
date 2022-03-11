#!/usr/bin/env bash
# Licensed to the Apache Software Foundation (ASF) under one or more
# contributor license agreements.  See the NOTICE file distributed with
# this work for additional information regarding copyright ownership.
# The ASF licenses this file to You under the Apache License, Version 2.0
# (the "License"); you may not use this file except in compliance with
# the License.  You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# exit immediately when a command fails
set -e
# only exit with zero if all commands of the pipeline exit successfully
set -o pipefail
# error on unset variables
set -u

show_help() {
cat << EOF
Usage: ./hack/release/version/propagate_version.sh [-h]

Make sure all files in the project reflect the version currently set in: version/version.go

    -h  Display this help and exit
EOF
}

OPTIND=1
while getopts h opt; do
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

# Get full version string
VERSION="$(cat version/version.go | grep -E 'Version([[:space:]]+)=' | grep -o '["''].*["'']' | xargs)"
VERSION_SUFFIX="$(cat version/version.go | grep -E 'VersionSuffix([[:space:]]+)=' | grep -o '["''].*["'']' | xargs)"
if [[ -n "${VERSION_SUFFIX:-}" ]]; then
  VERSION="${VERSION}-${VERSION_SUFFIX}"
fi

echo "Updating the version throughout the repo to: ${VERSION}"

# Update default solr-operator version and the helm chart versions.
awk '$1 == "repository:" { tag = ($2 == "apache/solr-operator") }
tag && $1 == "tag:"{$1 = "  " $1; $2 = "'"${VERSION}"'"} 1' helm/solr-operator/values.yaml > helm/solr-operator/values.yaml.tmp
mv helm/solr-operator/values.yaml.tmp helm/solr-operator/values.yaml

# Update Helm Chart.yaml
IS_PRE_RELEASE="false"
if [[ "${VERSION_SUFFIX}" =~ .*prerelease ]]; then
  IS_PRE_RELEASE="true"
fi

# Update Solr Operator Helm Chart version
{
  cat helm/solr-operator/Chart.yaml | \
  awk '$0 ~ /^v/ && $1 == "version:"{$1 = $1; $2 = "'"${VERSION#v}"'"} 1' | \
  awk '$0 ~ /^a/ && $1 == "appVersion:"{$1 = $1; $2 = "'"${VERSION}"'"} 1' | \
  awk '$1 == "artifacthub.io/prerelease:"{$1 = "  "$1; $2 = "\"'"${IS_PRE_RELEASE}"'\""} 1' | \
  sed -E "s|image: apache/solr-operator:(.*)|image: apache/solr-operator:${VERSION}|g"
} > helm/solr-operator/Chart.yaml.tmp && mv helm/solr-operator/Chart.yaml.tmp helm/solr-operator/Chart.yaml

# Update Solr Helm Chart version
{
  cat helm/solr/Chart.yaml | \
  awk '$0 ~ /^v/ && $1 == "version:"{$1 = $1; $2 = "'"${VERSION#v}"'"} 1' | \
  awk '$1 == "artifacthub.io/prerelease:"{$1 = "  "$1; $2 = "\"'"${IS_PRE_RELEASE}"'\""} 1'
} > helm/solr/Chart.yaml.tmp && mv helm/solr/Chart.yaml.tmp helm/solr/Chart.yaml


# Update Solr Operator Helm README.md
{
  cat helm/solr-operator/README.md | \
  sed -E 's/^\| image.tag \| string \| `".*"` \|/\| image.tag \| string \| `"'${VERSION}'"` \|/g' | \
  sed -E "s|^(kubectl.+/crds/)[^/<]+|\1${VERSION}|g" | \
  sed -E "s|^(helm.+--version )[^ <]+|\1${VERSION#v}|g"
} > helm/solr-operator/README.md.tmp && mv helm/solr-operator/README.md.tmp helm/solr-operator/README.md

# Update Solr Helm README.md
{
  cat helm/solr/README.md | \
  sed -E "s|^(helm.+--version )[^ <]+|\1${VERSION#v}|g"
} > helm/solr/README.md.tmp && mv helm/solr/README.md.tmp helm/solr/README.md


# Update Docs (Remove this when docs are generated with versioning info)
{
  cat docs/local_tutorial.md | \
  sed -E "s|(kubectl.+/crds/)[^/<]+|\1${VERSION}|g" | \
  sed -E "s|(helm.+--version )[^ <]+|\1${VERSION#v}|g"
} > docs/local_tutorial.md.tmp && mv docs/local_tutorial.md.tmp docs/local_tutorial.md
{
  cat docs/upgrade-notes.md | \
  sed -E "s|(kubectl.+/crds/)[^/<]+|\1${VERSION}|g" | \
  sed -E "s|(helm.+--version )[^ <]+|\1${VERSION#v}|g"
} > docs/upgrade-notes.md.tmp && mv docs/upgrade-notes.md.tmp docs/upgrade-notes.md
{
  cat docs/running-the-operator.md | \
  sed -E "s|(kubectl.+/crds/)[^/<]+|\1${VERSION}|g" | \
  sed -E "s|(helm.+--version )[^ <]+|\1${VERSION#v}|g"
} > docs/running-the-operator.md.tmp && mv docs/running-the-operator.md.tmp docs/running-the-operator.md

make manifests
