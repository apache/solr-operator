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
Usage: ./hack/release/smoke_test/verify_all.sh [-h] [-g GPG_KEY] -v VERSION -l LOCATION

Verify checksums and signatures of all release artifacts.
Check that the docker image contains the necessary LICENSE and NOTICE.

    -h  Display this help and exit
    -v  Version of the Solr Operator
    -l  Base location of the staged artifacts. Can be a URL or relative or absolute file path.
    -g  GPG Key (fingerprint) used to sign the artifacts. (Optional, if missing then fingerprints will not be checked)
EOF
}

OPTIND=1
while getopts hv:l:g: opt; do
    case $opt in
        h)
            show_help
            exit 0
            ;;
        v)  VERSION=$OPTARG
            ;;
        l)  LOCATION=$OPTARG
            ;;
        g)  GPG_KEY=$OPTARG
            ;;
        *)
            show_help >&2
            exit 1
            ;;
    esac
done
shift "$((OPTIND-1))"   # Discard the options and sentinel --

if [[ -z "${VERSION:-}" ]]; then
  echo "Specify a project version through -v, or through the VERSION env var" >&2 && exit 1
fi
if [[ -z "${LOCATION:-}" ]]; then
  echo "Specify an base artifact location through -l, or through the LOCATION env var" >&2 && exit 1
fi

TMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/solr-operator-smoke-verify-XXXXXX")

# If LOCATION is not a URL, then get the absolute path
if ! (echo "${LOCATION}" | grep "http"); then
  LOCATION=$(cd "${LOCATION}"; pwd)
fi

echo "Import Solr Keys"
curl -sL0 "https://dist.apache.org/repos/dist/release/solr/KEYS" | gpg --import --quiet


# First generate the old-style public key ring, if it doesn't already exist and contain the information we want.
# Only do this if a GPG Key was provided
if [[ -n "${GPG_KEY:-}" ]]; then
  if ! (gpg --no-default-keyring --keyring=~/.gnupg/pubring.gpg --list-keys "${GPG_KEY}"); then
    gpg --export >~/.gnupg/pubring.gpg
  fi
fi

echo "Download all artifacts and verify signatures"
# Do all logic in temporary directory
(
  cd "${TMP_DIR}"

  if (echo "${LOCATION}" | grep "http"); then
    # Download Source files from the staged location
    wget -r -np -nH -nd --level=1 "${LOCATION}/"

    # Download Helm files from the staged location
    wget -r -np -nH -nd --level=1 -P "helm-charts" "${LOCATION}/helm-charts/"

    # Download CRD files from the staged location
    wget -r -np -nH -nd --level=1 -P "crds" "${LOCATION}/crds/"
  else
    cp -r "${LOCATION}/"* .
  fi

  for artifact_directory in $(find * -type d -maxdepth 0); do
    (
      cd "${artifact_directory}"

      for artifact in $(find * -type f -maxdepth 0 ! \( -name '*.asc' -o -name '*.sha512' -o -name '*.prov' -o -name '*.html*' -o -name '*.txt*' \) ); do
        echo "Veryifying: ${artifact_directory}/${artifact}"
        sha512sum -c "${artifact}.sha512" \
          || { echo "Invalid sha512 for ${artifact}. Aborting!"; exit 1; }
        if [[ -n "${GPG_KEY:-}" ]]; then
          gpg --verify "${artifact}.asc" "${artifact}" \
            || { echo "Invalid signature for ${artifact}. Aborting!"; exit 1; }
        fi
      done

      # If a helm chart has a provenance file, and a GPG Key was provided, then verify the provenance file
      if [[ -f "${artifact}.prov" && -n "${GPG_KEY:-}" ]]; then
        helm verify "${artifact}"
      fi
    )
  done

  for artifact in $(find * -type f -maxdepth 0 ! \( -name '*.asc' -o -name '*.sha512' -o -name '*.prov' -o -name '*.html*' -o -name '*.txt*' \) ); do
    echo "Veryifying: ${artifact}"
    sha512sum -c "${artifact}.sha512" \
      || { echo "Invalid sha512 for ${artifact}. Aborting!"; exit 1; }
    if [[ -n "${GPG_KEY:-}" ]]; then
      gpg --verify "${artifact}.asc" "${artifact}" \
        || { echo "Invalid signature for ${artifact}. Aborting!"; exit 1; }
    fi
  done
)

# Delete temporary source download directory
rm -rf "${TMP_DIR}"

printf "\n********************\nSuccessfully verified all artifacts!\n\n"
