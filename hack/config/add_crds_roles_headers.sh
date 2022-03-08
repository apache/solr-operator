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

echo "Add headers to CRDs and Role files"

rm -f "${CONFIG_DIRECTORY:-config}"/crd/bases/*.tmp "${CONFIG_DIRECTORY:-config}"/rbac/role.yaml.tmp

files=("${CONFIG_DIRECTORY:-config}"/crd/bases/* "${CONFIG_DIRECTORY:-config}"/rbac/role.yaml)

# Copy and package CRDs
for file in "${files[@]}"; do
  {
    cat hack/headers/header.yaml.txt
    printf "\n"
    cat "${file}"
  } > "${file}.tmp" && mv "${file}.tmp" "${file}"
done
