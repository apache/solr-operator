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
Usage: ./hack/release/artifacts/set_signing_key.sh -f GPG_FINGERPRINT [-h]

Set the GPG Signing Key fingerprint where it needs to be set before a release.
Requires:
 * yq

    -h  Display this help and exit
    -f  Full GPG Key fingerprint to use when signing artifacts
EOF
}

OPTIND=1
while getopts hf: opt; do
    case $opt in
        f)  GPG_FINGERPRINT=$OPTARG
            ;;
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
if [[ -z "${GPG_FINGERPRINT:-}" ]]; then
  echo "Specify an GPG Signing key fingerprint -f, or through the GPG_FINGERPRINT env var" >&2 && exit 1
fi

echo "Setting the GPG Signing key fingerprint throughout the repo."

SIGNING_KEY_SET=".annotations.\"artifacthub.io/signKey\" |= \"fingerprint: ${GPG_FINGERPRINT}
url: https://dist.apache.org/repos/dist/release/solr/KEYS
\""

# Set Signing key for ArtifactHub in Chart.yaml
yq -i eval "${SIGNING_KEY_SET}" helm/solr-operator/Chart.yaml

yq -i eval "${SIGNING_KEY_SET}" helm/solr/Chart.yaml
