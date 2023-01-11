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

WORKING_DIR="$(pwd -P)"

cd "${PROJ_DIR}"

# Add all possible entry points for tests
if [[ "${WORKING_DIR}" == "${PROJ_DIR}/tests/e2e"* ]]; then
  RAW_GINKGO_TMP=("${@:1}")
  RAW_GINKGO=$(IFS=$'\036'; echo "${RAW_GINKGO_TMP[*]}")
  export RAW_GINKGO
  make e2e-tests
else
  # All other tests will be treated as unit tests
  make idea
  KUBEBUILDER_ASSETS="$(make kubebuilder-assets)"
  export KUBEBUILDER_ASSETS
  cd -
  export GINKGO_EDITOR_INTEGRATION=true
  "${PROJ_DIR}/bin/ginkgo" "$@"
fi
