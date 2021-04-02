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
Usage: ./hack/release/smoke_test/test_cluster.sh [-h] [-i IMAGE] -v VERSION -l LOCATION

Test the release candidate in a Kind cluster

    -h  Display this help and exit
    -v  Version of the Solr Operator
    -i  Solr Operator docker image to use  (Optional, defaults to apache/solr-operator:<version>)
    -l  Base location of the staged artifacts. Can be a URL or relative or absolute file path.
EOF
}

OPTIND=1
while getopts hv:i:l: opt; do
    case $opt in
        h)
            show_help
            exit 0
            ;;
        v)  VERSION=$OPTARG
            ;;
        i)  IMAGE=$OPTARG
            ;;
        l)  LOCATION=$OPTARG
            ;;
        *)
            show_help >&2
            exit 1
            ;;
    esac
done
shift "$((OPTIND-1))"   # Discard the options and sentinel --

if [[ -z "${VERSION:-}" ]]; then
  echo "Specify a project version through -v, or through the VERSION env var" >&2 && exit 1
fi
if [[ -z "${IMAGE:-}" ]]; then
  IMAGE="apache/solr-operator:${VERSION}"
fi
if [[ -z "${LOCATION:-}" ]]; then
  echo "Specify an base artifact location -l, or through the LOCATION env var" >&2 && exit 1
fi

# If LOCATION is not a URL, then get the absolute path
if ! (echo "${LOCATION}" | grep -E "http"); then
  LOCATION=$(cd "${LOCATION}"; pwd)
  LOCATION=${LOCATION%%/}

  HELM_CHART="${LOCATION}/helm-charts/solr-operator-${VERSION#v}.tgz"
else
  # If LOCATION is a URL, then we want to make sure we have the up-to-date docker image.
  docker pull "${IMAGE}"

  # Add the Test Helm Repo
  helm repo add --force-update "apache-solr-test-${VERSION}" "${LOCATION}/helm-charts"

  HELM_CHART="apache-solr-test-${VERSION}/solr-operator"
fi

if ! (which kind); then
  echo "Install Kind (Kubernetes in Docker)"
  GO111MODULE="on" go install sigs.k8s.io/kind@v0.10.0
fi

CLUSTER_NAME="solr-operator-${VERSION}-rc"
KUBE_CONTEXT="kind-${CLUSTER_NAME}"

if (kind get clusters | grep "${CLUSTER_NAME}"); then
  printf "Delete cluster, so the test starts with a clean slate.\n\n"
  kind delete clusters "${CLUSTER_NAME}"
fi

echo "Create test Kubernetes cluster in Kind. This will allow us to test the CRDs, Helm chart and the Docker image."
kind create cluster --name "${CLUSTER_NAME}"

# Load the docker image into the cluster
kind load docker-image --name "${CLUSTER_NAME}" "${IMAGE}"

# First generate the temporary public key ring
gpg --export >~/.gnupg/pubring.gpg

# Install the Solr Operator
kubectl create -f "${LOCATION}/crds/all-with-dependencies.yaml" || kubectl replace -f "${LOCATION}/crds/all-with-dependencies.yaml"
helm install --kube-context "${KUBE_CONTEXT}" --verify solr-operator "${HELM_CHART}" --set image.tag="${IMAGE##*:}" --set image.repository="${IMAGE%%:*}" --set image.pullPolicy="Never"

printf "\nInstall a test Solr Cluster\n"
cat <<EOF | kubectl apply -f -
apiVersion: solr.apache.org/v1beta1
kind: SolrCloud
metadata:
  name: example
spec:
  replicas: 3
  solrImage:
    tag: 8.7.0
  solrJavaMem: "-Xms1g -Xmx3g"
  customSolrKubeOptions:
    podOptions:
      resources:
        limits:
          memory: "1G"
        requests:
          cpu: "65m"
          memory: "156Mi"
  zookeeperRef:
    provided:
      persistence:
        spec:
          resources:
            requests:
              storage: "5Gi"
      replicas: 1
EOF

# Wait for solrcloud to be ready
printf '\nWait for all 3 Solr nodes to become ready.\n\n'
grep -q "3              3       3            3" <(exec kubectl get solrcloud example -w); kill $!

# Expose the common Solr service to localhost
kubectl port-forward service/example-solrcloud-common 8983:80 || true &
sleep 2

printf "\nCheck the admin URL to make sure it works\n"
curl --silent "http://localhost:8983/solr/admin/info/system" | grep '"status":0' > /dev/null

printf "\nCreating a test collection\n"
curl --silent "http://localhost:8983/solr/admin/collections?action=CREATE&name=smoke-test&replicationFactor=2&numShards=1" | grep '"status":0' > /dev/null

printf "\nQuery the test collection, test for 0 docs\n"
curl --silent "http://localhost:8983/solr/smoke-test/select" | grep '\"numFound\":0' > /dev/null

printf "\nCreate a Solr Prometheus Exporter to expose metrics for the Solr Cloud\n"
cat <<EOF | kubectl apply -f -
apiVersion: solr.apache.org/v1beta1
kind: SolrPrometheusExporter
metadata:
  name: example
spec:
  solrReference:
    cloud:
      name: "example"
  numThreads: 4
  image:
    tag: 8.7.0
EOF

printf "\nWait for the Solr Prometheus Exporter to be ready\n"
sleep 5
kubectl rollout status deployment/example-solr-metrics

# Expose the Solr Prometheus Exporter service to localhost
kubectl port-forward service/example-solr-metrics 8984:80 || true &
sleep 15

printf "\nQuery the prometheus exporter, test for 'http://example-solrcloud-0.example-solrcloud-headless.default:8983/solr' URL being scraped.\n"
curl --silent "http://localhost:8984/metrics" | grep http://example-solrcloud-0.example-solrcloud-headless.default:8983/solr > /dev/null

# If LOCATION is a URL, then remove the helm repo at the end
if (echo "${LOCATION}" | grep -E "http"); then
  helm repo remove "apache-solr-test-${VERSION}"
fi


echo "Delete test Kind Kubernetes cluster."
kind delete clusters "${CLUSTER_NAME}"

printf "\n********************\nLocal end-to-end cluster test successfully run!\n\n"
