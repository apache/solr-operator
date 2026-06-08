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

# Generates docs/antora.yml from docs/antora.template.yml, deriving the Antora
# component version from version/version.go. This keeps the Antora version in
# lock-step with the operator version without requiring Gradle (unlike Solr).
#
#   v0.10.0 (suffix "")           -> version '0_10', display_version '0.10'
#   v0.10.0 (suffix "prerelease") -> version '0_10', display_version '0.10-prerelease', prerelease: -prerelease

set -eu
set -o pipefail

# Resolve the project root (this script lives in hack/docs/).
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"

VERSION_GO="${PROJECT_DIR}/version/version.go"
TEMPLATE="${PROJECT_DIR}/docs/antora.template.yml"
OUTPUT="${PROJECT_DIR}/docs/antora.yml"

# Parse Version (e.g. v0.10.0) and VersionSuffix (e.g. prerelease) from version.go.
RAW_VERSION="$(grep -E 'Version([[:space:]]+)=' "${VERSION_GO}" | head -1 | sed -E 's/.*["'"'"']([^"'"'"']*)["'"'"'].*/\1/')"
SUFFIX="$(grep -E 'VersionSuffix([[:space:]]+)=' "${VERSION_GO}" | sed -E 's/.*["'"'"']([^"'"'"']*)["'"'"'].*/\1/')"

# Strip leading 'v' and split into semver parts.
SEMVER="${RAW_VERSION#v}"
IFS='.' read -r MAJOR MINOR PATCH <<< "${SEMVER}"

ANTORA_VERSION="${MAJOR}_${MINOR}"
DISPLAY_VERSION="${MAJOR}.${MINOR}"
OPERATOR_VERSION="${SEMVER}"

if [[ -n "${SUFFIX}" ]]; then
  DISPLAY_VERSION="${DISPLAY_VERSION}-${SUFFIX}"
  PRERELEASE_LINE="prerelease: -${SUFFIX}"
else
  PRERELEASE_LINE=""
fi

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
