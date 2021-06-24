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
Usage: ./hack/release/version/remove_version_specific_info.sh [-h]

Remove information in the project specific to a previous version.
Requires:
 * yq

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

echo "Removing information specific to the previous release"

CHANGE_LIST_REMOVAL='.annotations."artifacthub.io/changes" |= "- kind: added
  description: Addition 1
  links:
    - name: Github Issue
      url: https://github.com/issue-url
- kind: changed
  description: Change 2
  links:
    - name: Github PR
      url: https://github.com/pr-url
"'

SIGNING_KEY_REMOVAL=".annotations.\"artifacthub.io/signKey\" |= \"fingerprint: <fingerprint>
url: https://dist.apache.org/repos/dist/release/solr/KEYS
\""

SECURITY_FIX_REMOVAL='.annotations."artifacthub.io/containsSecurityUpdates" = "false"'

# Reset ArtifactHub info in Chart.yaml
yq -i eval "${CHANGE_LIST_REMOVAL}" helm/solr-operator/Chart.yaml
yq -i eval "${SIGNING_KEY_REMOVAL}" helm/solr-operator/Chart.yaml
yq -i eval "${SECURITY_FIX_REMOVAL}" helm/solr-operator/Chart.yaml

yq -i eval "${CHANGE_LIST_REMOVAL}" helm/solr/Chart.yaml
yq -i eval "${SIGNING_KEY_REMOVAL}" helm/solr/Chart.yaml
yq -i eval "${SECURITY_FIX_REMOVAL}" helm/solr/Chart.yaml
