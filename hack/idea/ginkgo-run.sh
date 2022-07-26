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

ENVTEST_ASSETS_DIR="${PROJ_DIR}/testbin"
mkdir -p "${ENVTEST_ASSETS_DIR}"
test -f "${ENVTEST_ASSETS_DIR}/setup-envtest.sh" || curl -sSLo "${ENVTEST_ASSETS_DIR}/setup-envtest.sh" https://raw.githubusercontent.com/kubernetes-sigs/controller-runtime/v0.8.3/hack/setup-envtest.sh
source "${ENVTEST_ASSETS_DIR}/setup-envtest.sh"
fetch_envtest_tools "${ENVTEST_ASSETS_DIR}"
setup_envtest_env "${ENVTEST_ASSETS_DIR}"
echo "${KUBEBUILDER_ASSETS}"
GINKGO_EDITOR_INTEGRATION=true "${PROJ_DIR}/bin/ginkgo" "$@"
