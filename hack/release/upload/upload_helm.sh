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
Usage: ./hack/release/upload/upload_helm.sh [-h] [-g GPG_KEY] -a APACHE_ID -c CHART_REPO -r RELEASE_URL

Upload the Helm Chart from staging to release.

    -h  Display this help and exit
    -c  Location of the Official Solr Operator Helm Chart repository. (The actual location, not the solr.apache.org/charts redirect)
    -r  URL of the Official Solr Operator release artifacts (do not include the 'helm-charts' suffix).
    -a  Apache ID
    -g  GPG Key to sign the new helm index file (Optional)
EOF
}

OPTIND=1
while getopts ha:g:c:r: opt; do
    case $opt in
        h)
            show_help
            exit 0
            ;;
        a)  APACHE_ID=$OPTARG
            ;;
        g)  GPG_KEY=$OPTARG
            ;;
        c)  CHART_REPO=$OPTARG
            ;;
        r)  RELEASE_URL=$OPTARG
            ;;
        *)
            show_help >&2
            exit 1
            ;;
    esac
done
shift "$((OPTIND-1))"   # Discard the options and sentinel --

if [[ -z "${CHART_REPO:-}" ]]; then
  echo "Specify a Helm Chart repository to upload to via -c, or through the CHART_REPO env var" >&2 && exit 1
fi
if [[ -z "${RELEASE_URL:-}" ]]; then
  echo "Specify a release url of the Solr Operator artifacts via -r, or through the RELEASE_URL env var" >&2 && exit 1
fi
if [[ -z "${APACHE_ID:-}" ]]; then
  echo "Specify your Apache ID for uploading via -a, or through the APACHE_ID env var" >&2 && exit 1
fi

GPG_USER=()
if [[ -n "${GPG_KEY:-}" ]]; then
  GPG_USER=(-u "${GPG_KEY}")
fi

export YAML_HEADER_FILE="$(pwd)/hack/headers/header.yaml.txt"

echo "Pulling Helm chart from the staged url and uploading to release Helm repo. ${CHART_REPO}"

# Put in a sub-shell so that the Password can't leak
(
  WITH_APACHE_ID=()
  if [[ -n "${APACHE_ID:-}" ]]; then
    printf "\nProvide your Apache Password, as it will be used to upload the CRDs to the apache nightlies server: "
    IFS= read -rs APACHE_PASSWORD < /dev/tty
    printf "\n\n"

    WITH_APACHE_ID=(-u "${APACHE_ID}:${APACHE_PASSWORD}")
    APACHE_PASSWORD=""
  fi

  TMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/solr-operator-release-helm-XXXXXXXX")

  # Do all logic in temporary directory
  (
    cd "${TMP_DIR}"

    # Pull Helm charts from staged location
    wget -r -np -nH -nd --level=1 -A "*.tgz*" "${RELEASE_URL}/helm-charts"

    rm -f robots.txt*

    # Pull the official Helm repo index.yaml
    wget "${CHART_REPO}/index.yaml"

    # Add the newly released Helm chart to the index
    helm repo index . --merge "index.yaml"

    {
      cat "${YAML_HEADER_FILE}"
      printf "\n\n"
      cat "index.yaml"
    } > "index.yaml.tmp" && mv "index.yaml.tmp" "index.yaml"

    # Generate signature and checksum for new index file
    gpg "${GPG_USER[@]}" --pinentry-mode loopback -ab "index.yaml"
    sha512sum -b "index.yaml" > "index.yaml.sha512"

    # Upload the newly released Helm charts
    for filename in *; do
      curl "${WITH_APACHE_ID[@]}" -T "${filename}" "${CHART_REPO}/"
    done
  )

  # Clear password, just in case
  WITH_APACHE_ID=()

  # Delete temporary artifact directory
  rm -rf "${TMP_DIR}"
)
