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

# Generates an Antora component descriptor (antora.yml) for the operator docs
# from docs/antora.template.yml, deriving the version from version/version.go.
#
# By default it writes the COMMITTED docs/antora.yml -- the descriptor the
# published Reference Guide uses for this branch. The release wizard regenerates
# it (via `make generate-antora-yaml`) at the steps where the published version
# changes; it is deliberately NOT regenerated on the post-release patch bump, so
# a release branch stays pinned to its last released version. Local preview
# builds (`make docs`) pass -o to write a throwaway descriptor into the
# build/staging directory, so the committed file is never modified during a
# normal build.
#
# Usage: generate_antora_yaml.sh [-o OUTPUT]
#   -o OUTPUT   output file (default: docs/antora.yml)
#
#   version.go v0.9.1                -> version '0_9',  display_version 'v0.9'
#   version.go v0.10.0 (prerelease)  -> version '0_10', display_version 'v0.10-prerelease', prerelease: -prerelease

set -eu
set -o pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
TEMPLATE="${PROJECT_DIR}/docs/antora.template.yml"
OUTPUT="${PROJECT_DIR}/docs/antora.yml"

while getopts ":o:" opt; do
  case "${opt}" in
    o) OUTPUT="${OPTARG}" ;;
    *) echo "Usage: $0 [-o OUTPUT]" >&2; exit 2 ;;
  esac
done

VERSION_GO="${PROJECT_DIR}/version/version.go"
RAW_VERSION="$(grep -E '^[[:space:]]*Version[[:space:]]' "${VERSION_GO}" | head -1 | sed -E 's/.*"([^"]*)".*/\1/')"
SUFFIX="$(grep -E '^[[:space:]]*VersionSuffix[[:space:]]' "${VERSION_GO}" | head -1 | sed -E 's/.*"([^"]*)".*/\1/')"

SEMVER="${RAW_VERSION#v}"
IFS='.' read -r MAJOR MINOR PATCH <<< "${SEMVER}"

ANTORA_VERSION="${MAJOR}_${MINOR}"
DISPLAY_VERSION="v${MAJOR}.${MINOR}"
OPERATOR_VERSION="${SEMVER}"

if [[ -n "${SUFFIX}" ]]; then
  DISPLAY_VERSION="${DISPLAY_VERSION}-${SUFFIX}"
  PRERELEASE_LINE="prerelease: -${SUFFIX}"
else
  PRERELEASE_LINE=""
fi

mkdir -p "$(dirname "${OUTPUT}")"
# Substitute tokens. Use '|' as the sed delimiter; values contain no '|'.
sed \
  -e "s|@ANTORA_VERSION@|${ANTORA_VERSION}|g" \
  -e "s|@DISPLAY_VERSION@|${DISPLAY_VERSION}|g" \
  -e "s|@OPERATOR_VERSION@|${OPERATOR_VERSION}|g" \
  "${TEMPLATE}" > "${OUTPUT}.tmp"

# Replace the prerelease placeholder line, or delete it entirely when released.
if [[ -n "${PRERELEASE_LINE}" ]]; then
  sed -e "s|@PRERELEASE_LINE@|${PRERELEASE_LINE}|g" "${OUTPUT}.tmp" > "${OUTPUT}"
else
  sed -e "/@PRERELEASE_LINE@/d" "${OUTPUT}.tmp" > "${OUTPUT}"
fi
rm -f "${OUTPUT}.tmp"

echo "Generated ${OUTPUT}: version '${ANTORA_VERSION}' (display '${DISPLAY_VERSION}')"
