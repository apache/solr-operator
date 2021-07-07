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

echo "Copying CRDs and Role to helm repo"

CONFIG_DIRECTORY="${CONFIG_DIRECTORY:-config}"
HELM_DIRECTORY="${HELM_DIRECTORY:-helm}"

# Copy and package CRDs
{
  cat hack/headers/header.yaml.txt
  printf "\n"
  cat "${CONFIG_DIRECTORY}/crd/bases/solr.apache.org_solrbackups.yaml"
  cat "${CONFIG_DIRECTORY}/crd/bases/solr.apache.org_solrclouds.yaml"
  cat "${CONFIG_DIRECTORY}/crd/bases/solr.apache.org_solrprometheusexporters.yaml"
} > "${HELM_DIRECTORY}/solr-operator/crds/crds.yaml"

# Copy Kube Role for Solr Operator permissions to Helm
# Template the Solr Operator role as needed for Helm values
{
  cat hack/headers/header.yaml.txt
  printf '\n\n{{- if .Values.rbac.create }}\n{{- range $namespace := (split "," (include "solr-operator.watchNamespaces" $)) }}\n'
  cat "${CONFIG_DIRECTORY}/rbac/role.yaml" \
    | awk '/^rules:$/{print "  namespace: {{ $namespace }}"}1' \
    | sed -E 's/^kind: ClusterRole$/kind: {{ include "solr-operator\.roleType" \$ }}/' \
    | sed -E 's/name: solr-operator-role$/name: {{ include "solr-operator\.fullname" \$ }}-role/'
  printf '\n{{- end }}\n{{- end }}\n'
} > "${HELM_DIRECTORY}/solr-operator/templates/role.yaml"
