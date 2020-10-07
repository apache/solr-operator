#!/usr/bin/env bash
# exit immediately when a command fails
set -e
# only exit with zero if all commands of the pipeline exit successfully
set -o pipefail
# error on unset variables
set -u

echo "Copying CRDs and Role to helm repo"

# Build and package CRDs
kubectl kustomize config/crd > helm/solr-operator/crds/crds.yaml

# Copy Kube Role for Solr Operator permissions to Helm
rm helm/solr-operator/templates/role.yaml
printf '{{- if .Values.rbac.create }}\n{{- range $namespace := (split "," (include "solr-operator.watchNamespaces" $)) }}\n' > helm/solr-operator/templates/role.yaml
cat config/rbac/role.yaml >> helm/solr-operator/templates/role.yaml
printf '\n{{- end }}\n{{- end }}' >> helm/solr-operator/templates/role.yaml
gawk -i inplace '/^rules:$/{print "  namespace: {{ $namespace }}"}1' helm/solr-operator/templates/role.yaml

# Template the Solr Operator role as needed
sed -i.bak -E 's/^kind: ClusterRole$/kind: {{ include "solr-operator\.roleType" \$ }}/' helm/solr-operator/templates/role.yaml
sed -i.bak -E 's/name: solr-operator-role$/name: {{ include "solr-operator\.fullname" \$ }}-role/' helm/solr-operator/templates/role.yaml
rm helm/solr-operator/templates/role.yaml.bak