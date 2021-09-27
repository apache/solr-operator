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

ZK_OP_VERSION="$(cat versions.props | grep -E 'zookeeper-operator' | grep -o 'v[[:digit:]]*\.[[:digit:]]*\.[[:digit:]]*')"

printf "Zookeeper Operator version is set to %s in versions.props, will check for compliance in all known places.\n\n" "${ZK_OP_VERSION}"

if (grep -E "https://github.com/pravega/zookeeper-operator/tree/" config/dependencies/zookeeper_cluster_crd.yaml | grep -v -E "${ZK_OP_VERSION}"); then
  printf "\nVersion not correct for all locations in file 'config/dependencies/zookeeper_cluster_crd.yaml' ^ \n" >&2 && exit 1
fi

if (grep -E "https://github.com/pravega/zookeeper-operator/tree/" hack/headers/zookeeper-operator-header.yaml.txt | grep -v -E "${ZK_OP_VERSION}"); then
  printf "\nVersion not correct for all locations in file 'hack/headers/zookeeper-operator-header.yaml.txt' ^ \n" >&2 && exit 1
fi

if (grep -E "https://github.com/pravega/zookeeper-operator/tree/" helm/solr-operator/README.md | grep -v -E "${ZK_OP_VERSION}"); then
  printf "\nVersion not correct for all locations in file 'helm/solr-operator/README.md' ^ \n" >&2 && exit 1
fi

if (grep -E "This was taken from https://github.com/pravega/zookeeper-operator" NOTICE | grep -v -E "${ZK_OP_VERSION}"); then
  printf "\nVersion not correct for all locations in file 'NOTICE' ^ \n" >&2 && exit 1
fi

if (grep -E "https://raw.githubusercontent.com/pravega/zookeeper-operator/" hack/release/artifacts/create_crds.sh | grep -v -E "${ZK_OP_VERSION}"); then
  printf "\nVersion not correct for all locations in file 'hack/release/artifacts/create_crds.sh' ^ \n" >&2 && exit 1
fi

if (grep -E "helm install zookeeper-operator" docs/development.md | grep -v -E "${ZK_OP_VERSION#v}"); then
  printf "\nVersion not correct for all locations in file 'docs/development.md' ^ \nMake sure that this version does not contain a 'v'.\n" >&2 && exit 1
fi

HELM_DEPENDENCY_VERSION="$(yq eval '.dependencies[] | select(.name = "zookeeper-operator") | .version' helm/solr-operator/Chart.yaml)"
if [[ ${HELM_DEPENDENCY_VERSION} != ${ZK_OP_VERSION#v} ]]; then
  printf "\nVersion not correct for Solr Operator Helm chart dependency, Zookeeper Operator version '%s' found.\nMake sure this version does not begin with a 'v'.\n" "${HELM_DEPENDENCY_VERSION}" >&2 && exit 1
fi