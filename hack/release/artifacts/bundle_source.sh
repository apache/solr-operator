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
Usage: ./hack/release/artifacts/bundle_source.sh [-h] [-v VERSION] -d ARTIFACTS_DIR

Setup the source release artifact.

    -h  Display this help and exit
    -v  Version of the Solr Operator (Optional, will default to project version)
    -d  Base directory of the staged artifacts.
EOF
}

OPTIND=1
while getopts hv:d: opt; do
    case $opt in
        h)
            show_help
            exit 0
            ;;
        v)  VERSION=$OPTARG
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

echo "Bundling source release for the Solr Operator ${VERSION} at '${ARTIFACTS_DIR}/solr-operator-${VERSION}.tgz'"

TAR="${TAR:=tar}"
if ! ("${TAR}" --version | grep "GNU tar"); then
  TAR="gtar"
  if ! ("${TAR}" --version | grep "GNU tar"); then
    printf "\nMust have a GNU tar installed. If on OSX, then please download gnu-tar. It is available via brew.\nThe GNU version of Tar must be available at either 'tar' or 'gtar'.\n"
    exit 1
  fi
fi

# Setup directory
mkdir -p "${ARTIFACTS_DIR}"
rm -rf "${ARTIFACTS_DIR}"/*.tgz*

# Package all of the code into a source release

COPYFILE_DISABLE=true "${TAR}" --exclude-vcs --exclude-vcs-ignores \
  --exclude .asf.yaml \
  --exclude .github \
  --exclude .run \
  --transform "s,^,solr-operator-${VERSION}/," \
  -czf "${ARTIFACTS_DIR}/solr-operator-${VERSION}.tgz" .
