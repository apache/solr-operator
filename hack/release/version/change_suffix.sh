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
Usage: ./hack/release/version/change_suffix.sh [-h] [-s SUFFIX]

Change the Version Suffix of the project.
If a suffix is not provided, the project suffix will flip between "" and "prerelease", depending on the current value.

    -h  Display this help and exit
    -s  New version suffix for the project. (Optional)
EOF
}

OPTIND=1
while getopts hs: opt; do
    case $opt in
        h)
            show_help
            exit 0
            ;;
        s)  VERSION_SUFFIX=$OPTARG
            ;;
        *)
            show_help >&2
            exit 1
            ;;
    esac
done
shift "$((OPTIND-1))"   # Discard the options and sentinel --

if [[ -z "${VERSION_SUFFIX:-}" ]]; then
  EXISTING_VERSION_SUFFIX="$(cat version/version.go | grep -E 'VersionSuffix([[:space:]]+)=' | grep -o '["''].*["'']' | xargs)"
  if [[ -z "${EXISTING_VERSION_SUFFIX}" ]]; then
    export VERSION_SUFFIX="prerelease"
  else
    export VERSION_SUFFIX=""
  fi
fi

echo "Updating the version suffix for the project to: ${VERSION_SUFFIX}"

# Version file
awk '$1 == "VersionSuffix"{$3 = "\"'"${VERSION_SUFFIX}"'\""} 1' version/version.go > version/version.tmp.go
go fmt version/version.tmp.go
mv version/version.tmp.go version/version.go
