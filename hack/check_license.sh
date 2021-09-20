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

licRes=$(
    find . -type f -iname '*.go' -not -name 'zkcluster.go' -not -name 'zkstatus.go' ! -exec \
         sh -c 'head -n5 $1 | grep -Eq "(Licensed to the Apache Software Foundation)" || echo -e  $1' {} {} \;
)

if [ -n "${licRes}" ]; then
	echo -e "license header checking failed:\\n${licRes}"
	exit 255
fi

licRes=$(
    find . -type f -iname '*.yaml' -not -regex '.*/config/manager/kustomization.yaml' -not -regex '.*/generated-check/.*' -not -name '*zookeeper*' ! -exec \
         sh -c 'head -n5 $1 | grep -Eq "(Licensed to the Apache Software Foundation)" || echo -e  $1' {} {} \;
)

if [ -n "${licRes}" ]; then
	echo -e "license header checking failed:\\n${licRes}"
	exit 255
fi

licRes=$(
    find . -type f -iname '*.sh' -not -regex '.*/testbin/.*'  -not -name 'build.sh' ! -exec \
         sh -c 'head -n5 $1 | grep -Eq "(Licensed to the Apache Software Foundation)" || echo -e  $1' {} {} \;
)

if [ -n "${licRes}" ]; then
	echo -e "license header checking failed:\\n${licRes}"
	exit 255
fi
