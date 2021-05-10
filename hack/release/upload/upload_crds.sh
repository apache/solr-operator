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
Usage: ./hack/release/upload/upload_crds.sh [-h] -a APACHE_ID -v VERSION -c CRDS_URL -r RELEASE_URL

Upload the CRDs from the artifacts release to a convenience location.

    -h  Display this help and exit
    -a  Apache ID
    -c  Convenience location to upload the released Solr Operator CRDs. Do NOT include the version at the end. (The actual location, not the solr.apache.org/operator/downloads/crds redirect)
    -r  URL of the Official Solr Operator release artifacts (do not include the 'helm-charts' suffix).
    -v  Version of this Solr Operator release.
EOF
}

OPTIND=1
while getopts ha:c:r:v: opt; do
    case $opt in
        h)
            show_help
            exit 0
            ;;
        a)  APACHE_ID=$OPTARG
            ;;
        c)  CRDS_URL=$OPTARG
            ;;
        r)  RELEASE_URL=$OPTARG
            ;;
        v)  VERSION=$OPTARG
            ;;
        *)
            show_help >&2
            exit 1
            ;;
    esac
done
shift "$((OPTIND-1))"   # Discard the options and sentinel --

if [[ -z "${CRDS_URL:-}" ]]; then
  echo "Specify a convenience CRDs URL through -c, or through the CRDS_URL env var" >&2 && exit 1
fi
if [[ -z "${RELEASE_URL:-}" ]]; then
  echo "Specify a release url of the Solr Operator artifacts via -r, or through the RELEASE_URL env var" >&2 && exit 1
fi
if [[ -z "${VERSION:-}" ]]; then
  echo "Specify a release version of the Solr Operator via -v, or through the VERSION env var" >&2 && exit 1
fi
if [[ -z "${APACHE_ID:-}" ]]; then
  echo "Specify your Apache ID for uploading via -a, or through the APACHE_ID env var" >&2 && exit 1
fi


CRDS_FULL_URL="${CRDS_URL%%/}/${VERSION}"

echo "Pulling CRDs from the staged url and uploading to release location ${CRDS_FULL_URL}."

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

  TMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/solr-operator-release-crds-XXXXXXXX")

  # Do all logic in temporary directory
  (
    cd "${TMP_DIR}"

    # Download CRD files from the staged location
    wget -r -np -nH -nd --level=1 -A "*.yaml*" "${RELEASE_URL}/crds/"

    rm -f robots.txt*

    # Create base release directory for CRDs
    curl "${WITH_APACHE_ID[@]}" -X MKCOL "${CRDS_FULL_URL}"

    # Upload each CRD file
    for filename in *; do
      curl "${WITH_APACHE_ID[@]}" -T "${filename}" "${CRDS_FULL_URL}/"
    done
  )

  # Clear password, just in case
  WITH_APACHE_ID=()

  # Delete temporary artifact directory
  rm -rf "${TMP_DIR}"
)
