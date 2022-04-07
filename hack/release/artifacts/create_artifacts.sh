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
Usage: ./hack/release/artifacts/create_artifacts.sh [-h] [-v VERSION] [-g GPG_KEY] [-a APACHE_ID] -d ARTIFACTS_DIR

Setup the release of all artifacts, then create signatures.

    -h  Display this help and exit
    -v  Version of the Solr Operator (Optional, will default to project version)
    -d  Base directory of the staged artifacts.
    -g  GPG Key to use when signing artifacts (Optional)
    -a  Apache ID, to use when signing the helm chart (Optional)
EOF
}

OPTIND=1
while getopts hv:g:a:d: opt; do
    case $opt in
        h)
            show_help
            exit 0
            ;;
        v)  VERSION=$OPTARG
            ;;
        g)  GPG_KEY=$OPTARG
            ;;
        a)  APACHE_ID=$OPTARG
            ;;
        d)  ARTIFACTS_DIR=$OPTARG
            ;;
        *)
            show_help >&2
            exit 1
            ;;
    esac
done
shift "$((OPTIND-1))"   # Discard the options and sentinel --

if [[ -z "${VERSION:-}" ]]; then
  VERSION=$(make -s version)
fi
if [[ -z "${ARTIFACTS_DIR:-}" ]]; then
  echo "Specify an base artifact directory -d, or through the ARTIFACTS_DIR env var" >&2 && exit 1
fi

GPG_USER=()
if [[ -n "${GPG_KEY:-}" ]]; then
  GPG_USER=(-u "${GPG_KEY}")
fi
APACHE_ID_PASS_THROUGH=()
if [[ -n "${APACHE_ID:-}" ]]; then
  APACHE_ID_PASS_THROUGH=(-a "${APACHE_ID}")
fi

echo "Setting up Solr Operator ${VERSION} release artifacts at '${ARTIFACTS_DIR}'"

./hack/release/artifacts/bundle_source.sh -d "${ARTIFACTS_DIR}" -v "${VERSION}"
./hack/release/artifacts/create_crds.sh -d "${ARTIFACTS_DIR}" -v "${VERSION}"
./hack/release/artifacts/build_helm.sh -d "${ARTIFACTS_DIR}" -v "${VERSION}" "${APACHE_ID_PASS_THROUGH[@]}"

# Generate signature and checksum for every file
(
  cd "${ARTIFACTS_DIR}"

  for artifact_directory in $(find * -type d -maxdepth 0); do
    (
      cd "${artifact_directory}"

      for artifact in $(find * -type f -maxdepth 0 ! \( -name '*.asc' -o -name '*.sha512' -o -name '*.prov' \) ); do
        echo "Signing ${artifact_directory}/${artifact}"
        if [[ -n "${GPG_KEY:-}" ]] && [ ! -f "${artifact}.asc" ]; then
          gpg -u "${GPG_KEY}" -ab "${artifact}"
        fi
        if [ ! -f "${artifact}.sha512" ]; then
          sha512sum -b "${artifact}" > "${artifact}.sha512"
        fi
      done
    )
  done

  for artifact in $(find * -type f -maxdepth 0 ! \( -name '*.asc' -o -name '*.sha512' -o -name '*.prov' \) ); do
    echo "Signing ${artifact}"
    if [[ -n "${GPG_KEY:-}" ]] && [ ! -f "${artifact}.asc" ]; then
      gpg -u "${GPG_KEY}" -ab "${artifact}"
    fi
    if [ ! -f "${artifact}.sha512" ]; then
      sha512sum -b "${artifact}" > "${artifact}.sha512"
    fi
  done
)
